package h2s

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"github.com/murakmii/c99-minimal-h2s/hpack"
	"net/http"
)

type (
	// ストリームの状態
	streamState uint8

	stream struct {
		state   streamState
		headers hpack.HeaderList
		body    []*frame
	}

	streamCollection struct {
		entries map[streamID]*stream
		maxID   streamID
	}
)

// idle, open, half closed(remote), closedの4状態を扱う
const (
	idleStream streamState = iota
	openStream
	halfClosedRemoteStream
	closedStream
)

// ある状態のストリームが、与えられたフレームを受信可能かどうかを判定する
func (s *stream) canAccept(f *frame) *h2Error {
	switch s.state {
	case idleStream:
		if f.typ != headersFrame {
			return newError(protocolError,
				"idle stream received frame %d", f.typ)
		}

	case openStream:
		return nil

	case halfClosedRemoteStream:
		if f.typ != windowUpdateFrame && f.typ != rstStreamFrame {
			return newError(streamClosedError,
				"half closed(remote) stream received frame %d", f.typ)
		}

	case closedStream:
		if f.typ != windowUpdateFrame && f.typ != rstStreamFrame {
			return newError(streamClosedError,
				"closed stream received frame %d", f.typ)
		}
	}

	return nil
}

func newStreamCollection() *streamCollection {
	return &streamCollection{
		entries: make(map[streamID]*stream), maxID: 0,
	}
}

// 全ストリーム中から指定IDのストリームを取得する。
// このメソッドはnilを返さない。仕様に基づき、指定IDがこれまでopenされた
// ストリームのIDより大きければ擬似的にidle状態のストリームを返し、
// そうでないなら実際にメモリ上に存在するストリームか、
// 擬似的にclosed状態のストリームを返す。
func (c *streamCollection) get(id streamID) *stream {
	if id <= c.maxID {
		s, ok := c.entries[id]
		if !ok {
			s = &stream{state: closedStream}
		}
		return s
	}
	return &stream{state: idleStream}
}

// ストリームをメモリ上に保存
func (c *streamCollection) save(id streamID, s *stream) {
	c.entries[id] = s
	if c.maxID < id {
		c.maxID = id
	}
}

// ストリームをclosed状態とする。
// closed状態のストリームを実際にメモリ上に保持しておく必要はないため、
// deleteにより削除しておく
func (c *streamCollection) close(id streamID) {
	delete(c.entries, id)
}

// multiplexerコンポーネントを表す構造体
type multiplexer struct {
	logger logger
	writer *writer

	in chan *frame

	indexTable *hpack.IndexTable
	streams    *streamCollection

	handler         http.Handler
	response        chan *responseWriter
	runningHandlers int
}

func newMultiplexer(
	logger logger,
	writer *writer,
	handler http.Handler,
) *multiplexer {
	return &multiplexer{
		logger: logger,
		writer: writer,
		in:     make(chan *frame),

		indexTable: hpack.NewIndexTable(4096),
		streams:    newStreamCollection(),
		handler:    handler,
		response:   make(chan *responseWriter),
	}
}

// 他のコンポーネントからフレームを渡す
func (mp *multiplexer) multiplex(f *frame) {
	mp.in <- f
}

// multiplexerコンポーネントの終了を指示
func (mp *multiplexer) shutdown() {
	close(mp.in)
}

// multiplexerコンポーネントの起動。
// 受け取ったフレームにより表現されるストリームとHTTPリクエストを処理する。
func (mp *multiplexer) run() {
	go func() {
		// multiplexerコンポーネントが処理を返す、
		// つまりwriterコンポーネントへ誰もフレームを渡さないことが
		// 確定してからそれの終了を指示する。
		defer func() {
			for mp.runningHandlers > 0 {
				mp.writeResponse(<-mp.response)
			}

			mp.writer.shutdown()
			mp.logger("multiplexer shutdown")
		}()

		for {
			select {
			case res := <-mp.response:
				mp.writeResponse(res)

			case f, ok := <-mp.in:
				if !ok {
					return
				}

				// エラーが発生した場合、PROTOCOL_ERRORなら
				// GOAWAYフレームにより接続を切断、それ以外のエラーなら
				// RST_STREAMフレームを送信しストリームをclosed状態とする。
				if f.streamID != 0 {
					s := mp.streams.get(f.streamID)
					if err := s.canAccept(f); err != nil {
						if err.code == protocolError {
							mp.writer.write(buildGoAwayFrame(err))
							return
						} else {
							mp.writer.write(
								buildRstStreamFrame(f.streamID, err))
							mp.streams.close(f.streamID)
							continue
						}
					}
				}

				switch f.typ {
				case dataFrame:
					// ペイロードをリクエストボディとしてストリームに紐付け保存する。
					// END_STREAMフラグが立っている場合、この時点で
					// HTTPリクエストの受信完了となるため、runHandlerメソッドにより
					// リクエストハンドラーを起動する。
					s := mp.streams.get(f.streamID)
					s.body = append(s.body, f)
					if f.flags.eos() {
						mp.runHandler(f.streamID, s)
					}

				case headersFrame:
					// HEADERSフレームなら、ペイロードを
					// ヘッダーブロックとしてデコードし、
					// 結果をリクエストヘッダーとしてストリームに紐付け保存する。
					// END_STREAMフラグが立っている場合、この時点で
					// HTTPリクエストの受信完了となるため、runHandlerメソッドにより
					// リクエストハンドラーを起動する。
					// フラグが立っていない場合open状態として保存し、
					// 後続のDATAフレームを待つ。
					headers, err := hpack.DecodeHeaderBlock(
						mp.indexTable,
						f.payload,
					)
					if err != nil {
						mp.writer.writeGoAway(compressionError,
							"failed to decode header block")
						return
					}

					s := mp.streams.get(f.streamID)
					s.headers = append(s.headers, headers...)
					if f.flags.eos() {
						mp.runHandler(f.streamID, s)
					} else {
						s.state = openStream
						mp.streams.save(f.streamID, s)
					}

				case rstStreamFrame:
					// クライアントからRST_STREAMを受信した場合、
					// 対象ストリームをclosed状態とする。
					code := binary.BigEndian.Uint32(f.payload)
					mp.logger("received RST_STREAM. code=%d", code)
					mp.streams.close(f.streamID)

				case settingsFrame:
					params := decodeSettingsParams(f)

					if value, ok := params[headerTableSizeSetting]; ok {
						mp.indexTable.UpdateAllowedTableSize(int(value))
					}

					mp.writer.changeSettings(params)

				case windowUpdateFrame:
					// ペイロードを加算するウィンドウサイズとしてデコードし、
					// writerコンポーネントに渡す
					size := int64(binary.BigEndian.Uint32(f.payload))
					mp.writer.incrWindow(f.streamID, size)
				}
			}
		}
	}()
}

func (mp *multiplexer) runHandler(id streamID, stream *stream) {
	// リクエストが生成出来ない場合はPROTOCOL_ERRORの
	// ストリームエラーを通知することとされている
	req, err := buildRequest(stream.headers, stream.body)
	if err != nil {
		mp.logger("(stream: %d) build request err %s", id, err)
		err = newError(protocolError, "request error")
		mp.writer.write(buildRstStreamFrame(id, err))
		mp.streams.close(id)
		return
	}

	stream.state = halfClosedRemoteStream
	mp.streams.save(id, stream)
	mp.runningHandlers++

	mp.logger("start http request processing. stream=%d", id)
	go func() {
		res := newResponseWriter(id)
		mp.handler.ServeHTTP(res, req)
		mp.response <- res
	}()
}

// リクエストヘッダーを表すヘッダーリストとリクエストボディを表すペイロードから、
// HTTP/1のリクエストを再現し、http.ReadRequest関数によりhttp.Request型の値を生成。
func buildRequest(
	headers hpack.HeaderList,
	bodies []*frame,
) (*http.Request, error) {
	http1Format := bytes.NewBuffer(nil)

	method := headers.Get(":method")
	authority := headers.Get(":authority")
	path := headers.Get(":path")

	if headers.Get("host") == nil {
		headers = append(
			headers,
			hpack.NewHeaderField("host", authority.Value()),
		)
	}

	// リクエスト行の書き出し
	reqLine := method.Value() + " " + path.Value() + " HTTP/1.1\r\n"
	http1Format.WriteString(reqLine)

	// 疑似ヘッダー以外のリクエストヘッダーの書き出し
	for _, hf := range headers {
		if hf.Name()[0] == ':' {
			continue
		}
		http1Format.WriteString(hf.String() + "\r\n")
	}

	http1Format.WriteString("\r\n")

	for _, body := range bodies {
		http1Format.Write(body.payload)
	}

	return http.ReadRequest(bufio.NewReader(http1Format))
}

// リクエストハンドラーからのレスポンスをフレームとして送信する
func (mp *multiplexer) writeResponse(res *responseWriter) {
	defer mp.streams.close(res.id)

	mp.runningHandlers--

	// リクエストハンドラーからレスポンスが生成された時点で
	// RST_STREAMフレーム等によりストリームが閉じていれば何もしない
	if mp.streams.get(res.id).state != halfClosedRemoteStream {
		return
	}

	for _, f := range res.buildFrames() {
		mp.writer.write(f)
	}
}

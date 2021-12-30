package h2s

import (
	"encoding/binary"
	"io"
)

type (
	// 他コンポーネントからウィンドウサイズの加算を
	// 通知する際に用いる構造体
	windowIncremented struct {
		id    streamID
		value int64
	}

	// writerコンポーネントを表す構造体
	writer struct {
		logger        logger
		peer          io.WriteCloser
		in            chan *frame
		settings      chan map[settingsParamType]uint32
		lastProcessed streamID
		maxFrameSize  int

		initWindow    int64
		window        chan *windowIncremented
		streamsWindow map[streamID]int64
		pendingData   []*frame
	}
)

func newWriter(logger logger, peer io.WriteCloser) *writer {
	return &writer{
		logger:       logger,
		peer:         peer,
		in:           make(chan *frame, 1),
		settings:     make(chan map[settingsParamType]uint32),
		maxFrameSize: 16384,

		initWindow:    65535,
		window:        make(chan *windowIncremented),
		streamsWindow: make(map[streamID]int64),
		pendingData:   make([]*frame, 0),
	}
}

// 他のコンポーネントからフレームを送信する
func (w *writer) write(f *frame) {
	w.in <- f
}

// GOAWAYフレーム送信のシンタックスシュガー
func (w *writer) writeGoAway(
	code errorCode,
	format string, a ...interface{},
) {
	w.write(buildGoAwayFrame(newError(code, format, a...)))
}

func (w *writer) changeSettings(params map[settingsParamType]uint32) {
	w.settings <- params
}

// ウィンドウサイズの加算をwriterコンポーネントに通知
func (w *writer) incrWindow(id streamID, value int64) {
	w.window <- &windowIncremented{id: id, value: value}
}

// writerコンポーネントの終了
func (w *writer) shutdown() {
	close(w.in)
}

// writerコンポーネントの起動。
// writeメソッドにより与えられたフレームを継続的にピアに送信する
func (w *writer) run() {
	defer w.logger("writer shutdown")

	w.write(&frame{
		typ: settingsFrame,
		payload: encodeSettingsParam([]*settingsParam{
			newSettingsParam(initialWindowSizeSetting, 2147483647),
		}),
	})

	// コネクションレベルのウィンドウサイズに初期ウィンドウサイズを設定。
	// ストリームID:0のストリームは存在しないため、
	// これをコネクションレベルのウィンドウサイズとして扱う。
	w.streamsWindow[0] = w.initWindow

	for {
		select {
		case f, ok := <-w.in:
			// shutdownメソッドにより終了が指示(チャネルがclose)されている場合
			// 接続を閉じて処理を返す
			if !ok {
				w.closePeer()
				return
			}

			switch f.typ {
			case dataFrame:
				// DATAフレームのフレームサイズに対して
				// ウィンドウサイズが少ない場合、DATAフレームを一旦退避させる。
				if _, ok := w.streamsWindow[f.streamID]; !ok {
					w.streamsWindow[f.streamID] = w.initWindow
				}

				pLen := int64(len(f.payload))
				if w.streamsWindow[0] < pLen ||
					w.streamsWindow[f.streamID] < pLen {
					w.pendingData = append(w.pendingData, f)
					continue
				}

			case goAwayFrame:
				binary.BigEndian.PutUint32(f.payload, uint32(w.lastProcessed))
			}

			w.sendToPeer(f)

		case incr := <-w.window:
			// 対象のウィンドウサイズを増加させ、
			// 退避されたDATAフレームの送信を試みる。
			if _, ok := w.streamsWindow[incr.id]; !ok {
				w.streamsWindow[incr.id] = w.initWindow
			}

			w.streamsWindow[incr.id] += incr.value
			w.logger("incremented window stream=%d, incr=%d",
				incr.id, incr.value)
			w.flushPendingData()

		case params := <-w.settings:
			if value, ok := params[initialWindowSizeSetting]; ok {
				// 初期ウィンドウサイズの変更を反映し、
				// 退避されたDATAフレームの送信を試みる。
				// 増分は新旧の差分である点に注意。
				diff := int64(value) - w.initWindow
				for k := range w.streamsWindow {
					w.streamsWindow[k] += diff
				}
				w.initWindow = int64(value)
				w.flushPendingData()
			}

			if value, ok := params[maxFrameSizeSetting]; ok {
				// 最大フレームサイズを記憶して送信時に適用
				w.maxFrameSize = int(value)
			}

			w.sendToPeer(&frame{typ: settingsFrame, flags: ackBit})
		}
	}

}

// ピアとの接続を1度だけ閉じる
func (w *writer) closePeer() {
	if w.peer == nil {
		return
	}
	w.peer.Close()
	w.peer = nil
	w.logger("close connection")
}

// 現在のウィンドウサイズを元に、退避されたDATAフレームを可能な限り送信する
func (w *writer) flushPendingData() {
	remain := make([]*frame, 0, len(w.pendingData))

	for _, data := range w.pendingData {
		dataLen := int64(len(data.payload))
		if w.streamsWindow[0] < dataLen ||
			w.streamsWindow[data.streamID] < dataLen {
			remain = append(remain, data)
			continue
		}

		w.sendToPeer(data)
	}

	w.pendingData = remain
}

// ピアにフレームを送信する
func (w *writer) sendToPeer(f *frame) {
	// ストリームの処理が終了している場合最終処理済みストリームIDを更新
	if f.isStreamCloser() && f.streamID > w.lastProcessed {
		w.lastProcessed = f.streamID
	}

	if w.peer == nil {
		return
	}

L:
	for _, f := range w.splitFrame(f) {
		if err := f.encodeTo(w.peer); err != nil {
			w.closePeer()
			return
		}

		switch f.typ {
		case dataFrame:
			// ピアへDATAフレームを送信できたので、
			// 各種ウィンドウサイズをからフレームサイズを減算。
			pLen := int64(len(f.payload))
			w.streamsWindow[0] -= pLen
			w.streamsWindow[f.streamID] -= pLen

		case goAwayFrame:
			w.logger("send GOAWAY. msg=%s", string(f.payload[8:]))
			w.closePeer()
			break L
		}
	}
}

// ペイロード長が最大フレームサイズを超過する場合に、
// 等価な複数のフレームに分割する。
func (w *writer) splitFrame(f *frame) []*frame {
	// DATA、HEADERSフレームでないか、最大フレームサイズ以下の
	// ペイロードなら何もしなくて良い。
	if (f.typ != dataFrame && f.typ != headersFrame) ||
		len(f.payload) <= w.maxFrameSize {
		return []*frame{f}
	}

	payloads := splitPayload(f.payload, w.maxFrameSize)
	frames := make([]*frame, 0, len(payloads))

	// HEADERSフレームの場合CONTINUATIONフレームで分割する
	fType := f.typ
	if f.typ == headersFrame {
		fType = continuationFrame
	}

	for _, p := range payloads {
		frames = append(frames, &frame{
			typ:      fType,
			streamID: f.streamID,
			payload:  p,
		})
	}

	// フラグを調整する。
	// DATAフレームの場合は単に分割した最後のフレームに元のフラグを設定する。
	// HEADERSフレームの場合、先頭のフレームのみHEADERSフレームとし、
	// そのフレームには元のフレームのEND_STREAMフラグを、
	// 最後のCONTINUATIONフレームには元のフレームのEND_HEADERフラグを設定する。
	if f.typ == dataFrame {
		frames[len(frames)-1].flags = f.flags
	} else {
		frames[0].typ = headersFrame
		frames[0].flags = f.flags & eosBit
		frames[len(frames)-1].flags = f.flags & eohBit
	}

	return frames
}

// バイト列 p をそれぞれの長さが size 以下のチャンクに分割する
func splitPayload(p []byte, size int) [][]byte {
	var chunk []byte
	chunks := make([][]byte, 0, len(p)/size+1)

	for len(p) > size {
		chunk, p = p[:size], p[size:]
		chunks = append(chunks, chunk)
	}

	if len(p) > 0 {
		chunks = append(chunks, p)
	}

	return chunks
}

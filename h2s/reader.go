package h2s

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
)

// フレームのペイロードの最大値。
// 仕様では初期値は@<code>{16384}と規定されている。
// 変更することは可能だが本誌では初期値のまま扱うため、
// 単に定数として定義する。
const maxFrameSize = 16384

var clientPreface = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

// readerコンポーネントの起動。
// フレームの受信とmultiplexerコンポーネントへの引き渡しを継続的に行う。
func runReader(
	logger logger,
	peer io.Reader,
	writer *writer,
	handler http.Handler,
) {
	go func() {
		multiplexer := newMultiplexer(logger, writer, handler)
		multiplexer.run()

		receivedPreface := make([]byte, len(clientPreface))
		if _, err := io.ReadFull(peer, receivedPreface); err != nil {
			logger("failed to read client preface: %s", err)
			return
		}

		if bytes.Compare(receivedPreface, clientPreface) != 0 {
			logger("invalid client preface")
			return
		}

		logger("connection preface completed")

		// readerコンポーネントが処理を返す、
		// つまりmultiplexerコンポーネントへ誰もフレームを渡さないことが
		// 確定してからそれの終了を指示する。
		defer func() {
			logger("reader shutdown")
			multiplexer.shutdown()
		}()

		var headerBuf []*frame

		for {
			// フレームの受信に失敗した場合はreaderコンポーネントを終了する。
			// HTTP/2関連のエラーであれば事前にGOAWAYフレームを送信する。
			f, err := readFrame(peer, maxFrameSize)
			if err != nil {
				if h2, ok := err.(*h2Error); ok {
					writer.write(buildGoAwayFrame(h2))
				} else {
					logger("failed to read frame: %s", err)
				}
				return
			}

			// 不完全なヘッダブロックがあるにも関わらず、
			// 当該ヘッダブロックのCONTINUATIONフレーム以外が来た場合はエラー
			if len(headerBuf) > 0 && f.typ != continuationFrame {
				writer.writeGoAway(protocolError, "invalid header sequence")
				return
			}

			// 不明なフレームタイプは単に無視することと仕様で規定されている
			if f.typ > continuationFrame {
				continue
			}

			// 各種フレームタイプについてフィルタ等を行った上で
			// multiplexerコンポーネントにフレームを渡す。
			switch f.typ {
			case headersFrame:
				if !f.flags.eoh() {
					headerBuf = append(headerBuf, f)
					continue
				}

			case priorityFrame:
				continue

			case settingsFrame:
				if f.flags.ack() {
					continue
				}

			case pushPromiseFrame:
				writer.writeGoAway(protocolError, "don't use push promise")
				return

			case pingFrame:
				if !f.flags.ack() {
					logger("received PING and respond ack")
					f.flags = ackBit
					writer.write(f)
				}
				continue

			case goAwayFrame:
				logger(
					"received GOAWAY. code=%d, msg(str)=%s",
					binary.BigEndian.Uint32(f.payload[4:]),
					string(f.payload[8:]),
				)
				return

			case continuationFrame:
				if len(headerBuf) == 0 || headerBuf[0].streamID != f.streamID {
					writer.writeGoAway(protocolError, "invalid header block")
					return
				}

				headerBuf = append(headerBuf, f)
				if f.flags.eoh() {
					f = mergeHeaders(headerBuf)
					headerBuf = nil
				}
			}

			multiplexer.multiplex(f)
		}
	}()
}

func mergeHeaders(frames []*frame) *frame {
	merged := &frame{
		typ:      headersFrame,
		flags:    (frames[0].flags & eosBit) | eohBit,
		streamID: frames[0].streamID,
	}

	for _, f := range frames[1:] {
		merged.payload = append(merged.payload, f.payload...)
	}

	return merged
}

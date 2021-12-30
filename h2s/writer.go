package h2s

import (
	"io"
)

type (
	// writerコンポーネントを表す構造体
	writer struct {
		logger logger
		peer   io.WriteCloser
		in     chan *frame
	}
)

func newWriter(logger logger, peer io.WriteCloser) *writer {
	return &writer{
		logger: logger,
		peer:   peer,
		in:     make(chan *frame, 1),
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

// writerコンポーネントの終了
func (w *writer) shutdown() {
	close(w.in)
}

// writerコンポーネントの起動。
// writeメソッドにより与えられたフレームを継続的にピアに送信する
func (w *writer) run() {
	defer w.logger("writer shutdown")

	w.write(&frame{
		typ:     settingsFrame,
		payload: encodeSettingsParam([]*settingsParam{}),
	})

	for {
		select {
		case f, ok := <-w.in:
			// shutdownメソッドにより終了が指示(チャネルがclose)されている場合
			// 接続を閉じて処理を返す
			if !ok {
				w.closePeer()
				return
			}

			w.sendToPeer(f)
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

// ピアにフレームを送信する
func (w *writer) sendToPeer(f *frame) {
	if w.peer == nil {
		return
	}

	if err := f.encodeTo(w.peer); err != nil {
		w.closePeer()
		return
	}

	// GOAWAYフレームを送信したなら切断する
	if f.typ == goAwayFrame {
		w.logger("send GOAWAY. msg=%s", string(f.payload[8:]))
		w.closePeer()
	}
}

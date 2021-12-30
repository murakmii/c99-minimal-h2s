package h2s

import (
	"encoding/binary"
	"fmt"
)

type (
	errorCode uint32

	h2Error struct {
		code errorCode
		msg  string
	}
)

var _ error = (*h2Error)(nil)

const (
	protocolError     errorCode = 0x01 // 様々なケースで用いられる汎用エラーコード
	internalError     errorCode = 0x02 // 予期せぬ内部エラー
	flowControlError  errorCode = 0x03 // フロー制御関連のエラー
	streamClosedError errorCode = 0x05 // ストリーム単位での不正なフレームの送信
	frameSizeError    errorCode = 0x06 // フレームサイズが不正
	compressionError  errorCode = 0x07 // ヘッダーの圧縮、つまりHPACK関連のエラー
)

// エラーコードを伴うエラーを生じさせる必要がある場合は今後この関数を用いる
func newError(code errorCode, format string, a ...interface{}) *h2Error {
	return &h2Error{code: code, msg: fmt.Sprintf(format, a...)}
}

func (e *h2Error) Error() string {
	return e.msg
}

// エラーからGOAWAYフレームを生成する
func buildGoAwayFrame(e error) *frame {
	// エラーがh2Errorでない場合はエラーコードが不明なので、内部エラーとしておく
	h2, ok := e.(*h2Error)
	if !ok {
		h2 = newError(internalError, "internal error")
	}

	f := &frame{
		typ:     goAwayFrame,
		payload: make([]byte, 8),
	}

	// ストリームIDは暫定的にゼロ値のままにしている点に注意
	binary.BigEndian.PutUint32(f.payload[4:], uint32(h2.code))
	f.payload = append(f.payload, h2.msg...)

	return f
}

// エラーからRST_STREAMフレームを生成する
func buildRstStreamFrame(id streamID, e error) *frame {
	code := internalError
	if h2, ok := e.(*h2Error); ok {
		code = h2.code
	}

	f := &frame{
		typ:      rstStreamFrame,
		streamID: id,
		payload:  make([]byte, 4),
	}

	binary.BigEndian.PutUint32(f.payload, uint32(code))
	return f
}

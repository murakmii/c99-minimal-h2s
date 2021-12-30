package h2s

import (
	"encoding/binary"
	"io"
)

type (
	frameType uint8  // フレームタイプ
	streamID  uint32 // ストリームID
	flags     uint8  // フラグ

	// フレームを表す構造体
	frame struct {
		typ      frameType
		flags    flags
		streamID streamID
		payload  []byte
	}
)

const (
	// フレームタイプを表す定数
	dataFrame         frameType = 0x00
	headersFrame      frameType = 0x01
	priorityFrame     frameType = 0x02
	rstStreamFrame    frameType = 0x03
	settingsFrame     frameType = 0x04
	pushPromiseFrame  frameType = 0x05
	pingFrame         frameType = 0x06
	goAwayFrame       frameType = 0x07
	windowUpdateFrame frameType = 0x08
	continuationFrame frameType = 0x09

	// フラグの各ビット
	eosBit      = 0x01
	ackBit      = eosBit
	eohBit      = 0x04
	paddedBit   = 0x08
	priorityBit = 0x20
)

// フラグ判定のためのビット演算を読みやすくするためのメソッド群
func (f flags) eos() bool {
	return f&eosBit > 0
}

func (f flags) ack() bool {
	return f.eos()
}

func (f flags) eoh() bool {
	return f&eohBit > 0
}

func (f flags) padded() bool {
	return f&paddedBit > 0
}

func (f flags) priority() bool {
	return f&priorityBit > 0
}

// 読み込み先からのフレームの読み込み。まずヘッダーを読み込み、
// そこから得られたペイロード長を元にペイロードを追加で読み込む。
//
// HTTP/2ではフレームサイズ(ペイロード長)の上限が設けられるため、
// 引数として与えられたそれをペイロード長が超える場合はエラーとする。
// この時のエラーはFRAME_SIZE_ERRORであることと規定されているため、
// newError関数によりこれを表現するエラーを生成して返す。
func readFrame(r io.Reader, maxFrameSize int) (*frame, error) {
	header := make([]byte, 9)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	f := &frame{
		typ:      frameType(header[3]),
		flags:    flags(header[4]),
		streamID: streamID(binary.BigEndian.Uint32(header[5:])),
	}

	pLen := int(header[0])<<16 | int(header[1])<<8 | int(header[2])
	if pLen > maxFrameSize {
		return nil,
			newError(frameSizeError, "too large payload(%d bytes)", pLen)
	}

	f.payload = make([]byte, pLen)
	if _, err := io.ReadFull(r, f.payload); err != nil {
		return nil, err
	}

	return normalizeFrame(f), nil
}

func normalizeFrame(f *frame) *frame {
	if f.typ != dataFrame && f.typ != headersFrame {
		return f
	}

	pLen := len(f.payload)

	if f.flags.padded() {
		f.flags &= ^flags(paddedBit)
		f.payload = f.payload[1 : pLen-int(f.payload[0])]
	}

	if f.typ == headersFrame && f.flags.priority() {
		f.flags &= ^flags(priorityBit)
		f.payload = f.payload[5:]
	}

	return f
}

// 与えられた出力先にフレームを書き出す
func (f *frame) encodeTo(w io.Writer) error {
	pLen := len(f.payload)
	header := make([]byte, 9)

	header[0] = byte((pLen >> 16) & 0xFF)
	header[1] = byte((pLen >> 8) & 0xFF)
	header[2] = byte(pLen & 0xFF)
	header[3] = byte(f.typ)
	header[4] = byte(f.flags)
	binary.BigEndian.PutUint32(header[5:], uint32(f.streamID))

	if _, err := w.Write(header); err != nil {
		return err
	}

	if _, err := w.Write(f.payload); err != nil {
		return err
	}

	return nil
}

type (
	// 設定の種別
	settingsParamType uint16

	// 設定を表す構造体
	settingsParam struct {
		typ   settingsParamType
		value uint32
	}
)

const (
	headerTableSizeSetting   settingsParamType = 0x01
	enablePushSetting        settingsParamType = 0x02
	maxConcurrentStreams     settingsParamType = 0x03
	initialWindowSizeSetting settingsParamType = 0x04
	maxFrameSizeSetting      settingsParamType = 0x05
	maxHeaderListSizeSetting settingsParamType = 0x06
)

func newSettingsParam(
	typ settingsParamType,
	value uint32,
) *settingsParam {
	return &settingsParam{typ: typ, value: value}
}

// 設定のエンコード
func encodeSettingsParam(params []*settingsParam) []byte {
	encoded := make([]byte, len(params)*6)
	for i, p := range params {
		binary.BigEndian.PutUint16(encoded[i*6:], uint16(p.typ))
		binary.BigEndian.PutUint32(encoded[i*6+2:], p.value)
	}
	return encoded
}

// 設定のデコード
func decodeSettingsParams(f *frame) map[settingsParamType]uint32 {
	// 設定1つのバイナリフォーマットは必ず6バイトなので、除算すれば数が分かる
	n := len(f.payload) / 6
	params := make(map[settingsParamType]uint32, n)

	for i := 0; i < n; i++ {
		typ := settingsParamType(binary.BigEndian.Uint16(f.payload[6*i:]))
		value := binary.BigEndian.Uint32(f.payload[6*i+2:])

		params[typ] = value
	}

	return params
}

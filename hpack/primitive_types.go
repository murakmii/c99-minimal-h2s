package hpack

import "fmt"

// ヘッダブロック block から prefix ビットプレフィックスの整数をデコードする。
// 戻り値としてデコードして得られた整数と未処理のヘッダブロックを返す。
func decodeInt(block []byte, prefix int) (uint64, []byte, error) {
	mask := uint64(1<<prefix - 1)
	prefixed := uint64(block[0]) & mask

	// Nビットプレフィックスの全ビットが1でないならこれをデコード結果として返す
	if prefixed < mask {
		return prefixed, block[1:], nil
	}

	var value uint64
	shift := 0

	for offset := 1; ; offset += 1 {
		// 整数表現は後続フラグがある限り無限に続けることができるが、
		// 64ビットで表現できる範囲を超える場合はエラーとする。
		if shift > 64 {
			return 0, nil, fmt.Errorf("invalid integer")
		}

		b := block[offset]

		// データの後続フラグである最上位1ビットは無視し、
		// これまでデコードした結果と足し合わせていく
		value += uint64(b&0x7F) << shift
		shift += 7

		// 後続フラグが立っていないなら終了。
		// Nビットプレフィックス値を加算することを忘れずに。
		if b&0x80 == 0 {
			return value + prefixed, block[offset+1:], nil
		}
	}
}

// 整数 i を prefix ビットプレフィックス整数としてエンコードし、
// 出力先 dst に追加する。
func encodeInt(dst []byte, i uint64, prefix int) []byte {
	// Nビットプレフィックスの最大値未満で表現できるならそのまま出力先に追加して終了
	mask := uint64(1<<prefix - 1)
	if i < uint64(1<<prefix-1) {
		return append(dst, byte(i))
	}

	i -= mask
	dst = append(dst, byte(mask))

	// 値を7ビットに分解しつつ、後続フラグと共に出力先に追加する
	// (0x80(128)未満になった時点で後続フラグは不要)
	for ; i >= 0x80; i >>= 7 {
		dst = append(dst, 0x80|byte(i&0x7F))
	}

	return append(dst, byte(i))
}

// ヘッダブロック block から文字列をデコードする。
// 戻り値として得られた文字列と未処理のヘッダブロックを返す。
func decodeStr(block []byte) (string, []byte, error) {
	compressed := (block[0] & 0x80) > 0
	strLen, remain, err := decodeInt(block, 7)
	if err != nil {
		return "", nil, err
	}

	str := remain[0:strLen]
	if compressed {
		if str, err = decodeHuffman(str); err != nil {
			return "", nil, err
		}
	}

	return string(str), remain[strLen:], nil
}

// 文字列 str をエンコードし出力先 dst に追加する。
// ハフマン符号による圧縮には対応しない。
func encodeStr(dst []byte, str string) []byte {
	dst = encodeInt(dst, uint64(len(str)), 7)
	return append(dst, []byte(str)...)
}

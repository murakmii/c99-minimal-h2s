package hpack

import "strings"

// ヘッダーフィールドの順序付けられたコレクションであるヘッダーリスト
type HeaderList []*HeaderField

// 名前が一致するヘッダーフィールドをヘッダーリストから取得する(ignore case)
func (hl HeaderList) Get(name string) *HeaderField {
	for _, hf := range hl {
		if strings.ToLower(hf.Name()) == strings.ToLower(name) {
			return hf
		}
	}

	return nil
}

// ヘッダーブロックをデコードし、ヘッダーリストを得る。
// デコードにはその最中に参照されるインデックステーブルが必要。
func DecodeHeaderBlock(t *IndexTable, block []byte) (HeaderList, error) {
	var err error
	var hf *HeaderField
	list := make([]*HeaderField, 0)

	// インデックスヘッダーフィールド、リテラルヘッダーフィールド
	// 最大テーブルサイズ更新を判断し、それぞれに応じたデコードや
	// インデックステーブルの更新を行う。
	// 上位ビットの仕様により、1バイト目を各種定数と比較することで、
	// どのバイナリフォーマットかが判断可。
	for len(block) > 0 {
		switch {
		case block[0] >= 0x80:
			// インデックスヘッダーフィールド
			hf, block, err = decodeIndexHeaderField(t, block)
			if err != nil {
				return nil, err
			}
			list = append(list, hf)

		case block[0] >= 0x40:
			// インデックス更新を伴うリテラルヘッダフィールド
			hf, block, err = decodeLiteralHeaderField(t, block, 6)
			if err != nil {
				return nil, err
			}
			list = append(list, hf)
			t.add(hf)

		case block[0] >= 0x20:
			// 最大テーブルサイズ更新
			var newSize uint64
			newSize, block, err = decodeInt(block, 5)
			if err := t.updateMaxTableSize(int(newSize)); err != nil {
				return nil, err
			}

		default:
			// 上記以外のリテラルヘッダーフィールド
			hf, block, err = decodeLiteralHeaderField(t, block, 4)
			if err != nil {
				return nil, err
			}
			list = append(list, hf)
		}
	}

	return list, nil
}

// インデックスヘッダーフィールドのデコード。
// 7ビットプレフィックス整数をデコードし、得られた整数をインデックスとして
// インデックステーブルからヘッダーフィールドを取得して返す。
func decodeIndexHeaderField(
	t *IndexTable,
	block []byte,
) (*HeaderField, []byte, error) {
	var err error
	var index uint64

	index, block, err = decodeInt(block, 7)
	if err != nil {
		return nil, nil, err
	}

	hf, err := t.get(int(index))
	if err != nil {
		return nil, nil, err
	}

	return hf, block, nil
}

// リテラルヘッダーフィールドのデコード。
// 引数 prefix で与えれたビット数のNビットプレフィックス整数をデコードし、
// 非ゼロならそれをインデックスとしてインデックステーブルから名前を、
// 続く文字列リテラル表現から値をデコードしヘッダーフィールドとして返す。
// Nビットプレフィックス整数が表す整数が0なら文字列リテラル表現を2つデコードし、
// それぞれを名前、値としたヘッダーフィールドを返す。
func decodeLiteralHeaderField(
	t *IndexTable,
	block []byte,
	prefix int,
) (*HeaderField, []byte, error) {
	var err error
	var index uint64

	index, block, err = decodeInt(block, prefix)
	if err != nil {
		return nil, nil, err
	}

	var nameOrVal string
	nameOrVal, block, err = decodeStr(block)
	if err != nil {
		return nil, nil, err
	}

	if index > 0 {
		hf, err := t.get(int(index))
		if err != nil {
			return nil, nil, err
		}
		return NewHeaderField(hf.Name(), nameOrVal), block, nil
	}

	var value string
	value, block, err = decodeStr(block)
	if err != nil {
		return nil, nil, err
	}
	return NewHeaderField(nameOrVal, value), block, nil
}

// ヘッダーリストをヘッダーブロックへエンコードする。
// 簡略化のため、ヘッダーフィールド1つ1つを必ず
// インデックスされないリテラルヘッダフィールドとしてエンコードする。
func EncodeHeaderList(list HeaderList) []byte {
	encoded := make([]byte, 0)
	for _, hf := range list {
		encoded = append(encoded, 0x10)
		encoded = encodeStr(encoded, hf.Name())
		encoded = encodeStr(encoded, hf.Value())
	}
	return encoded
}

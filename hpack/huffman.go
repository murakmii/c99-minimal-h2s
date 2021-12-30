package hpack

import "fmt"

type (
	// ハフマン符号化用テーブルの1行を表すための構造体
	huffmanCode struct {
		code    uint
		bitsLen int
	}

	// デコード用二分木のノードを表す構造体
	huffmanNode struct {
		sym  *byte           // 文字が割り当てられている場合、そのASCIIコード
		next [2]*huffmanNode // 子ノードへの参照
	}
)

// デコード用二分木の根
var huffmanRoot = &huffmanNode{next: [2]*huffmanNode{nil, nil}}

// ハフマン符号化用テーブルを元にデコード用二分木を構築する
func buildTree(entry []*huffmanCode) {
	for sym, e := range entry {
		// 符号の上位ビットから順番に走査し、デコード用二分木を辿り、
		// 文字を割り当てるべきノードを得る。
		node := huffmanRoot
		for shift := e.bitsLen - 1; shift >= 0; shift-- {
			b := (e.code >> shift) & 0x01
			if node.next[b] == nil {
				node.next[b] = &huffmanNode{
					next: [2]*huffmanNode{nil, nil},
				}
			}

			node = node.next[b]
		}

		byteSym := byte(sym)
		node.sym = &byteSym
	}
}

// ハフマン符号によりエンコードされた文字列をデコードし伸長する。
// デコード対象を先頭から1ビットずつ走査し、その値通りにデコード用二分木を辿っていく。
// 文字が割り当てられているノードを見つけると1文字分のデコード完了となる。
func decodeHuffman(compressed []byte) ([]byte, error) {
	decoded := make([]byte, 0)
	node := huffmanRoot

	// 末尾が1でパディングされていることを確かめるための変数
	var validPadding byte = 0x01

	for i := 0; i < len(compressed); i++ {
		for shift := 7; shift >= 0; shift-- {
			bit := (compressed[i] >> shift) & 0x01
			node = node.next[bit]
			if node == nil {
				return nil, fmt.Errorf("invalid huffman code")
			}

			if node.sym != nil {
				// 文字が割り当てられていれば1文字デコード完了。また根から辿り直す
				decoded = append(decoded, *node.sym)
				node = huffmanRoot
				validPadding = 0x01
			} else {
				validPadding &= bit
			}
		}
	}

	// デコードが完了した際に値1を持つビットが連続した状態で終了していなければ、
	// 不正なパディングと見なしエラーを返す。
	if validPadding != 0x01 {
		return nil, fmt.Errorf("invalid padding")
	}

	return decoded, nil
}

// プロセス起動時に1度だけデコード用二分木を構築する
func init() {
	buildTree([]*huffmanCode{
		{code: 0x1ff8, bitsLen: 13},
		{code: 0x7fffd8, bitsLen: 23},
		{code: 0xfffffe2, bitsLen: 28},
		{code: 0xfffffe3, bitsLen: 28},
		{code: 0xfffffe4, bitsLen: 28},
		{code: 0xfffffe5, bitsLen: 28},
		{code: 0xfffffe6, bitsLen: 28},
		{code: 0xfffffe7, bitsLen: 28},
		{code: 0xfffffe8, bitsLen: 28},
		{code: 0xffffea, bitsLen: 24},
		{code: 0x3ffffffc, bitsLen: 30},
		{code: 0xfffffe9, bitsLen: 28},
		{code: 0xfffffea, bitsLen: 28},
		{code: 0x3ffffffd, bitsLen: 30},
		{code: 0xfffffeb, bitsLen: 28},
		{code: 0xfffffec, bitsLen: 28},
		{code: 0xfffffed, bitsLen: 28},
		{code: 0xfffffee, bitsLen: 28},
		{code: 0xfffffef, bitsLen: 28},
		{code: 0xffffff0, bitsLen: 28},
		{code: 0xffffff1, bitsLen: 28},
		{code: 0xffffff2, bitsLen: 28},
		{code: 0x3ffffffe, bitsLen: 30},
		{code: 0xffffff3, bitsLen: 28},
		{code: 0xffffff4, bitsLen: 28},
		{code: 0xffffff5, bitsLen: 28},
		{code: 0xffffff6, bitsLen: 28},
		{code: 0xffffff7, bitsLen: 28},
		{code: 0xffffff8, bitsLen: 28},
		{code: 0xffffff9, bitsLen: 28},
		{code: 0xffffffa, bitsLen: 28},
		{code: 0xffffffb, bitsLen: 28},
		{code: 0x14, bitsLen: 6},
		{code: 0x3f8, bitsLen: 10},
		{code: 0x3f9, bitsLen: 10},
		{code: 0xffa, bitsLen: 12},
		{code: 0x1ff9, bitsLen: 13},
		{code: 0x15, bitsLen: 6},
		{code: 0xf8, bitsLen: 8},
		{code: 0x7fa, bitsLen: 11},
		{code: 0x3fa, bitsLen: 10},
		{code: 0x3fb, bitsLen: 10},
		{code: 0xf9, bitsLen: 8},
		{code: 0x7fb, bitsLen: 11},
		{code: 0xfa, bitsLen: 8},
		{code: 0x16, bitsLen: 6},
		{code: 0x17, bitsLen: 6},
		{code: 0x18, bitsLen: 6},
		{code: 0x0, bitsLen: 5},
		{code: 0x1, bitsLen: 5},
		{code: 0x2, bitsLen: 5},
		{code: 0x19, bitsLen: 6},
		{code: 0x1a, bitsLen: 6},
		{code: 0x1b, bitsLen: 6},
		{code: 0x1c, bitsLen: 6},
		{code: 0x1d, bitsLen: 6},
		{code: 0x1e, bitsLen: 6},
		{code: 0x1f, bitsLen: 6},
		{code: 0x5c, bitsLen: 7},
		{code: 0xfb, bitsLen: 8},
		{code: 0x7ffc, bitsLen: 15},
		{code: 0x20, bitsLen: 6},
		{code: 0xffb, bitsLen: 12},
		{code: 0x3fc, bitsLen: 10},
		{code: 0x1ffa, bitsLen: 13},
		{code: 0x21, bitsLen: 6},
		{code: 0x5d, bitsLen: 7},
		{code: 0x5e, bitsLen: 7},
		{code: 0x5f, bitsLen: 7},
		{code: 0x60, bitsLen: 7},
		{code: 0x61, bitsLen: 7},
		{code: 0x62, bitsLen: 7},
		{code: 0x63, bitsLen: 7},
		{code: 0x64, bitsLen: 7},
		{code: 0x65, bitsLen: 7},
		{code: 0x66, bitsLen: 7},
		{code: 0x67, bitsLen: 7},
		{code: 0x68, bitsLen: 7},
		{code: 0x69, bitsLen: 7},
		{code: 0x6a, bitsLen: 7},
		{code: 0x6b, bitsLen: 7},
		{code: 0x6c, bitsLen: 7},
		{code: 0x6d, bitsLen: 7},
		{code: 0x6e, bitsLen: 7},
		{code: 0x6f, bitsLen: 7},
		{code: 0x70, bitsLen: 7},
		{code: 0x71, bitsLen: 7},
		{code: 0x72, bitsLen: 7},
		{code: 0xfc, bitsLen: 8},
		{code: 0x73, bitsLen: 7},
		{code: 0xfd, bitsLen: 8},
		{code: 0x1ffb, bitsLen: 13},
		{code: 0x7fff0, bitsLen: 19},
		{code: 0x1ffc, bitsLen: 13},
		{code: 0x3ffc, bitsLen: 14},
		{code: 0x22, bitsLen: 6},
		{code: 0x7ffd, bitsLen: 15},
		{code: 0x3, bitsLen: 5},
		{code: 0x23, bitsLen: 6},
		{code: 0x4, bitsLen: 5},
		{code: 0x24, bitsLen: 6},
		{code: 0x5, bitsLen: 5},
		{code: 0x25, bitsLen: 6},
		{code: 0x26, bitsLen: 6},
		{code: 0x27, bitsLen: 6},
		{code: 0x6, bitsLen: 5},
		{code: 0x74, bitsLen: 7},
		{code: 0x75, bitsLen: 7},
		{code: 0x28, bitsLen: 6},
		{code: 0x29, bitsLen: 6},
		{code: 0x2a, bitsLen: 6},
		{code: 0x7, bitsLen: 5},
		{code: 0x2b, bitsLen: 6},
		{code: 0x76, bitsLen: 7},
		{code: 0x2c, bitsLen: 6},
		{code: 0x8, bitsLen: 5},
		{code: 0x9, bitsLen: 5},
		{code: 0x2d, bitsLen: 6},
		{code: 0x77, bitsLen: 7},
		{code: 0x78, bitsLen: 7},
		{code: 0x79, bitsLen: 7},
		{code: 0x7a, bitsLen: 7},
		{code: 0x7b, bitsLen: 7},
		{code: 0x7ffe, bitsLen: 15},
		{code: 0x7fc, bitsLen: 11},
		{code: 0x3ffd, bitsLen: 14},
		{code: 0x1ffd, bitsLen: 13},
		{code: 0xffffffc, bitsLen: 28},
		{code: 0xfffe6, bitsLen: 20},
		{code: 0x3fffd2, bitsLen: 22},
		{code: 0xfffe7, bitsLen: 20},
		{code: 0xfffe8, bitsLen: 20},
		{code: 0x3fffd3, bitsLen: 22},
		{code: 0x3fffd4, bitsLen: 22},
		{code: 0x3fffd5, bitsLen: 22},
		{code: 0x7fffd9, bitsLen: 23},
		{code: 0x3fffd6, bitsLen: 22},
		{code: 0x7fffda, bitsLen: 23},
		{code: 0x7fffdb, bitsLen: 23},
		{code: 0x7fffdc, bitsLen: 23},
		{code: 0x7fffdd, bitsLen: 23},
		{code: 0x7fffde, bitsLen: 23},
		{code: 0xffffeb, bitsLen: 24},
		{code: 0x7fffdf, bitsLen: 23},
		{code: 0xffffec, bitsLen: 24},
		{code: 0xffffed, bitsLen: 24},
		{code: 0x3fffd7, bitsLen: 22},
		{code: 0x7fffe0, bitsLen: 23},
		{code: 0xffffee, bitsLen: 24},
		{code: 0x7fffe1, bitsLen: 23},
		{code: 0x7fffe2, bitsLen: 23},
		{code: 0x7fffe3, bitsLen: 23},
		{code: 0x7fffe4, bitsLen: 23},
		{code: 0x1fffdc, bitsLen: 21},
		{code: 0x3fffd8, bitsLen: 22},
		{code: 0x7fffe5, bitsLen: 23},
		{code: 0x3fffd9, bitsLen: 22},
		{code: 0x7fffe6, bitsLen: 23},
		{code: 0x7fffe7, bitsLen: 23},
		{code: 0xffffef, bitsLen: 24},
		{code: 0x3fffda, bitsLen: 22},
		{code: 0x1fffdd, bitsLen: 21},
		{code: 0xfffe9, bitsLen: 20},
		{code: 0x3fffdb, bitsLen: 22},
		{code: 0x3fffdc, bitsLen: 22},
		{code: 0x7fffe8, bitsLen: 23},
		{code: 0x7fffe9, bitsLen: 23},
		{code: 0x1fffde, bitsLen: 21},
		{code: 0x7fffea, bitsLen: 23},
		{code: 0x3fffdd, bitsLen: 22},
		{code: 0x3fffde, bitsLen: 22},
		{code: 0xfffff0, bitsLen: 24},
		{code: 0x1fffdf, bitsLen: 21},
		{code: 0x3fffdf, bitsLen: 22},
		{code: 0x7fffeb, bitsLen: 23},
		{code: 0x7fffec, bitsLen: 23},
		{code: 0x1fffe0, bitsLen: 21},
		{code: 0x1fffe1, bitsLen: 21},
		{code: 0x3fffe0, bitsLen: 22},
		{code: 0x1fffe2, bitsLen: 21},
		{code: 0x7fffed, bitsLen: 23},
		{code: 0x3fffe1, bitsLen: 22},
		{code: 0x7fffee, bitsLen: 23},
		{code: 0x7fffef, bitsLen: 23},
		{code: 0xfffea, bitsLen: 20},
		{code: 0x3fffe2, bitsLen: 22},
		{code: 0x3fffe3, bitsLen: 22},
		{code: 0x3fffe4, bitsLen: 22},
		{code: 0x7ffff0, bitsLen: 23},
		{code: 0x3fffe5, bitsLen: 22},
		{code: 0x3fffe6, bitsLen: 22},
		{code: 0x7ffff1, bitsLen: 23},
		{code: 0x3ffffe0, bitsLen: 26},
		{code: 0x3ffffe1, bitsLen: 26},
		{code: 0xfffeb, bitsLen: 20},
		{code: 0x7fff1, bitsLen: 19},
		{code: 0x3fffe7, bitsLen: 22},
		{code: 0x7ffff2, bitsLen: 23},
		{code: 0x3fffe8, bitsLen: 22},
		{code: 0x1ffffec, bitsLen: 25},
		{code: 0x3ffffe2, bitsLen: 26},
		{code: 0x3ffffe3, bitsLen: 26},
		{code: 0x3ffffe4, bitsLen: 26},
		{code: 0x7ffffde, bitsLen: 27},
		{code: 0x7ffffdf, bitsLen: 27},
		{code: 0x3ffffe5, bitsLen: 26},
		{code: 0xfffff1, bitsLen: 24},
		{code: 0x1ffffed, bitsLen: 25},
		{code: 0x7fff2, bitsLen: 19},
		{code: 0x1fffe3, bitsLen: 21},
		{code: 0x3ffffe6, bitsLen: 26},
		{code: 0x7ffffe0, bitsLen: 27},
		{code: 0x7ffffe1, bitsLen: 27},
		{code: 0x3ffffe7, bitsLen: 26},
		{code: 0x7ffffe2, bitsLen: 27},
		{code: 0xfffff2, bitsLen: 24},
		{code: 0x1fffe4, bitsLen: 21},
		{code: 0x1fffe5, bitsLen: 21},
		{code: 0x3ffffe8, bitsLen: 26},
		{code: 0x3ffffe9, bitsLen: 26},
		{code: 0xffffffd, bitsLen: 28},
		{code: 0x7ffffe3, bitsLen: 27},
		{code: 0x7ffffe4, bitsLen: 27},
		{code: 0x7ffffe5, bitsLen: 27},
		{code: 0xfffec, bitsLen: 20},
		{code: 0xfffff3, bitsLen: 24},
		{code: 0xfffed, bitsLen: 20},
		{code: 0x1fffe6, bitsLen: 21},
		{code: 0x3fffe9, bitsLen: 22},
		{code: 0x1fffe7, bitsLen: 21},
		{code: 0x1fffe8, bitsLen: 21},
		{code: 0x7ffff3, bitsLen: 23},
		{code: 0x3fffea, bitsLen: 22},
		{code: 0x3fffeb, bitsLen: 22},
		{code: 0x1ffffee, bitsLen: 25},
		{code: 0x1ffffef, bitsLen: 25},
		{code: 0xfffff4, bitsLen: 24},
		{code: 0xfffff5, bitsLen: 24},
		{code: 0x3ffffea, bitsLen: 26},
		{code: 0x7ffff4, bitsLen: 23},
		{code: 0x3ffffeb, bitsLen: 26},
		{code: 0x7ffffe6, bitsLen: 27},
		{code: 0x3ffffec, bitsLen: 26},
		{code: 0x3ffffed, bitsLen: 26},
		{code: 0x7ffffe7, bitsLen: 27},
		{code: 0x7ffffe8, bitsLen: 27},
		{code: 0x7ffffe9, bitsLen: 27},
		{code: 0x7ffffea, bitsLen: 27},
		{code: 0x7ffffeb, bitsLen: 27},
		{code: 0xffffffe, bitsLen: 28},
		{code: 0x7ffffec, bitsLen: 27},
		{code: 0x7ffffed, bitsLen: 27},
		{code: 0x7ffffee, bitsLen: 27},
		{code: 0x7ffffef, bitsLen: 27},
		{code: 0x7fffff0, bitsLen: 27},
		{code: 0x3ffffee, bitsLen: 26},
	})
}

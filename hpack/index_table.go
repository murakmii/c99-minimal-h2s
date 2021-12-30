package hpack

import "fmt"

type (
	// ヘッダーフィールド
	HeaderField struct {
		name  string
		value string
	}
)

var (
	// 静的テーブル
	staticTable    []*HeaderField
	staticTableLen int
)

func NewHeaderField(name, value string) *HeaderField {
	return &HeaderField{name: name, value: value}
}

func (hf *HeaderField) Name() string {
	return hf.name
}

func (hf *HeaderField) Value() string {
	return hf.value
}

func (hf *HeaderField) String() string {
	return hf.Name() + ": " + hf.Value()
}

// ヘッダーフィールドのサイズを計算するメソッド
func (hf *HeaderField) Size() int {
	return len(hf.name) + len(hf.value) + 32
}

// インデックステーブル
type IndexTable struct {
	allowedTableSize int // 最大テーブルサイズの上限
	maxTableSize     int // 最大テーブルサイズ
	tableSize        int // 現在のテーブルサイズ
	dynamicTable     []*HeaderField
}

// 最大テーブルサイズを指定してインデックステーブルを生成
func NewIndexTable(allowedTableSize int) *IndexTable {
	return &IndexTable{
		allowedTableSize: allowedTableSize,
		maxTableSize:     allowedTableSize,
		tableSize:        0,
		dynamicTable:     []*HeaderField{},
	}
}

// 最大テーブルサイズの上限を更新。
// これを呼び出すのはHPACKを使用する、より上位のプロトコル(つまりHTTP/2)
func (t *IndexTable) UpdateAllowedTableSize(size int) {
	t.allowedTableSize = size
	if t.maxTableSize > t.allowedTableSize {
		t.maxTableSize = t.allowedTableSize
	}
	t.evict()
}

// 最大テーブルサイズを更新
func (t *IndexTable) updateMaxTableSize(size int) error {
	if size > t.allowedTableSize {
		return fmt.Errorf("invalid max data size")
	}

	t.maxTableSize = size
	t.evict()
	return nil
}

// 動的テーブルへのヘッダーフィールドの追加。
// HPACKの仕様では最後に追加されたヘッダーフィールドに最も小さいインデックスが
// 与えられるが、Goではスライスの先頭への要素の追加は非効率であるため、
// appendにより末尾に追加して動的テーブルを再現する。
func (t *IndexTable) add(h *HeaderField) {
	t.dynamicTable = append(t.dynamicTable, h)
	t.tableSize += h.Size()
	t.evict()
}

// インデックスを指定してインデックステーブルが管理するヘッダーフィールドを取得。
// addメソッドの説明の通り、最後に追加されたヘッダーフィールドが末尾に存在しているため、
// 例えば動的テーブルの先頭を指すインデックスならスライスの末尾のヘッダーフィールドを
// 返すといった変換を渡されたインデックスに対して行う。
func (t *IndexTable) get(index int) (*HeaderField, error) {
	if index > 0 && index <= staticTableLen {
		return staticTable[index-1], nil
	}

	dIdx := len(t.dynamicTable) - (index - staticTableLen)
	if dIdx < 0 {
		return nil, fmt.Errorf("invalid index")
	}

	return t.dynamicTable[dIdx], nil
}

// 動的テーブルが保持しているヘッダーフィールドの合計サイズが
// 動的テーブルサイズを超えるなら、最も古いヘッダーフィールドから順に削除する。
// 私たちの実装では、最も古いヘッダーフィールドとはスライスの先頭なので、
// 先頭から順番に超過分を削除していく。
func (t *IndexTable) evict() {
	drop := 0
	for t.tableSize > t.maxTableSize {
		t.tableSize -= t.dynamicTable[drop].Size()
		drop += 1
	}

	if drop == 0 {
		return
	}

	copy(t.dynamicTable, t.dynamicTable[drop:])
	for i := 1; i <= drop; i++ {
		t.dynamicTable[len(t.dynamicTable)-i] = nil
	}
}

// プロセス起動時に静的テーブルを1度だけ構築。
// 構築と言っても、単に HeaderField のスライスで初期化するのみ。
func init() {
	staticTable = []*HeaderField{
		NewHeaderField(":authority", ""),
		NewHeaderField(":method", "GET"),
		NewHeaderField(":method", "POST"),
		NewHeaderField(":path", "/"),
		NewHeaderField(":path", "/index.html"),
		NewHeaderField(":scheme", "http"),
		NewHeaderField(":scheme", "https"),
		NewHeaderField(":status", "200"),
		NewHeaderField(":status", "204"),
		NewHeaderField(":status", "206"),
		NewHeaderField(":status", "304"),
		NewHeaderField(":status", "400"),
		NewHeaderField(":status", "404"),
		NewHeaderField(":status", "500"),
		NewHeaderField("accept-charset", ""),
		NewHeaderField("accept-encoding", "gzip, deflate"),
		NewHeaderField("accept-language", ""),
		NewHeaderField("accept-ranges", ""),
		NewHeaderField("accept", ""),
		NewHeaderField("access-control-allow-origin", ""),
		NewHeaderField("age", ""),
		NewHeaderField("allow", ""),
		NewHeaderField("authorization", ""),
		NewHeaderField("cache-control", ""),
		NewHeaderField("content-disposition", ""),
		NewHeaderField("content-encoding", ""),
		NewHeaderField("content-language", ""),
		NewHeaderField("content-length", ""),
		NewHeaderField("content-location", ""),
		NewHeaderField("content-range", ""),
		NewHeaderField("content-type", ""),
		NewHeaderField("cookie", ""),
		NewHeaderField("date", ""),
		NewHeaderField("etag", ""),
		NewHeaderField("expect", ""),
		NewHeaderField("expires", ""),
		NewHeaderField("from", ""),
		NewHeaderField("host", ""),
		NewHeaderField("if-match", ""),
		NewHeaderField("if-modified-since", ""),
		NewHeaderField("if-none-match", ""),
		NewHeaderField("if-range", ""),
		NewHeaderField("if-unmodified-since", ""),
		NewHeaderField("last-modified", ""),
		NewHeaderField("link", ""),
		NewHeaderField("location", ""),
		NewHeaderField("max-forwards", ""),
		NewHeaderField("proxy-authenticate", ""),
		NewHeaderField("proxy-authorization", ""),
		NewHeaderField("range", ""),
		NewHeaderField("referer", ""),
		NewHeaderField("refresh", ""),
		NewHeaderField("retry-after", ""),
		NewHeaderField("server", ""),
		NewHeaderField("set-cookie", ""),
		NewHeaderField("strict-transport-security", ""),
		NewHeaderField("transfer-encoding", ""),
		NewHeaderField("user-agent", ""),
		NewHeaderField("vary", ""),
		NewHeaderField("via", ""),
		NewHeaderField("www-authenticate", ""),
	}
	staticTableLen = len(staticTable)
}

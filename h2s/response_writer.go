package h2s

import (
	"bytes"
	"github.com/murakmii/c99-minimal-h2s/hpack"
	"net/http"
	"strconv"
	"strings"
)

// http.ResponseWriterインターフェイスを満たす構造体
type responseWriter struct {
	id            streamID
	header        http.Header
	statusCode    int
	writtenHeader hpack.HeaderList
	body          *bytes.Buffer
}

var _ http.ResponseWriter = (*responseWriter)(nil)

func newResponseWriter(id streamID) *responseWriter {
	return &responseWriter{id: id, header: make(http.Header)}
}

// Headerメソッドの実装。
// 単にHeader(実体はmap)を返す。
func (res *responseWriter) Header() http.Header {
	return res.header
}

// レスポンスボディの書き出し。
// この時点では単にバッファするのみ。
func (res *responseWriter) Write(b []byte) (int, error) {
	res.WriteHeader(200)

	if res.body == nil {
		res.body = bytes.NewBuffer(nil)
	}

	return res.body.Write(b)
}

// レスポンスヘッダーの書き出し。
// この時点で設定されているヘッダーをヘッダーリストとして確定させる。
func (res *responseWriter) WriteHeader(statusCode int) {
	if res.writtenHeader != nil {
		return
	}

	res.statusCode = statusCode
	res.writtenHeader = make(hpack.HeaderList, 0, len(res.header)+1)

	res.writtenHeader = append(res.writtenHeader,
		hpack.NewHeaderField(":status", strconv.Itoa(statusCode)))

	for key, values := range res.header {
		key = strings.ToLower(key)
		for _, value := range values {
			res.writtenHeader = append(res.writtenHeader,
				hpack.NewHeaderField(key, value))
		}
	}
}

// 設定されたレスポンスの内容を等価な一連のフレームに変換する
func (res *responseWriter) buildFrames() []*frame {
	res.WriteHeader(200)

	body := res.body.Bytes()
	bodyLen := len(body)

	// http.ResponseWriterの要件通り、
	// http.DetectContentTypeによってContent-Typeを決定。
	if res.writtenHeader.Get("content-type") == nil {
		res.writtenHeader = append(
			res.writtenHeader,
			hpack.NewHeaderField(
				"content-type",
				http.DetectContentType(body),
			),
		)
	}

	if res.writtenHeader.Get("content-length") == nil {
		res.writtenHeader = append(
			res.writtenHeader,
			hpack.NewHeaderField(
				"content-length",
				strconv.Itoa(bodyLen),
			),
		)
	}

	frames := []*frame{
		{
			typ:      headersFrame,
			flags:    eohBit,
			streamID: res.id,
			payload:  hpack.EncodeHeaderList(res.writtenHeader),
		},
	}

	// レスポンスボディが無いなら
	// HEADERSフレームにEND_STREAMフラグを設定し終了
	if bodyLen == 0 {
		frames[0].flags |= eosBit
		return frames
	}

	return append(frames, &frame{
		typ:      dataFrame,
		flags:    eosBit,
		streamID: res.id,
		payload:  body,
	})
}

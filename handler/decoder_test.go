package handler

import (
	"API_Gateway/builder"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
	"bufio"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// readTestRequest 把一段原始 HTTP 请求字节解析成 *http.Request，用于构造 decoder 输入。
func readTestRequest(t *testing.T, raw string) *http.Request {
	t.Helper()
	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	return req
}

// TestDecoderDecodeFields 验证 Decode 把 *http.Request 的各字段正确映射进 message.Request。
func TestDecoderDecodeFields(t *testing.T) {
	d := NewDecoder(builder.DecoderConfig{MaxBody: 4096})
	req := readTestRequest(t,
		"POST /api/users?id=1 HTTP/1.1\r\n"+
			"Host: example.com\r\n"+
			"X-Custom: foo\r\n"+
			"Content-Length: 5\r\n"+
			"\r\n"+
			"hello")

	got, err := d.Decode(req)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Method != "POST" {
		t.Errorf("Method = %q, want %q", got.Method, "POST")
	}
	if got.Proto != "HTTP/1.1" {
		t.Errorf("Proto = %q, want %q", got.Proto, "HTTP/1.1")
	}
	if got.Host != "example.com" {
		t.Errorf("Host = %q, want %q", got.Host, "example.com")
	}
	if string(got.Body) != "hello" {
		t.Errorf("Body = %q, want %q", got.Body, "hello")
	}
	if got.ContentLength != 5 {
		t.Errorf("ContentLength = %d, want 5", got.ContentLength)
	}
	if got.URL == nil {
		t.Fatal("URL is nil")
	}
	if got.URL.Path != "/api/users" {
		t.Errorf("URL.Path = %q, want %q", got.URL.Path, "/api/users")
	}
	if got.URL.Query().Get("id") != "1" {
		t.Errorf("URL query id = %q, want %q", got.URL.Query().Get("id"), "1")
	}
	if got.Header.Get("X-Custom") != "foo" {
		t.Errorf("Header X-Custom = %q, want %q", got.Header.Get("X-Custom"), "foo")
	}
}

// TestDecoderBodyTooLarge 请求体超过 MaxBody 时返回哨兵错误 ErrBodyTooLarge。
func TestDecoderBodyTooLarge(t *testing.T) {
	d := NewDecoder(builder.DecoderConfig{MaxBody: 4})
	req := readTestRequest(t,
		"POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhello")

	_, err := d.Decode(req)
	if !errors.Is(err, gerrors.ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
}

// TestDecoderDefaultMaxBody MaxBody 未配置（<=0）时回落到默认值。
func TestDecoderDefaultMaxBody(t *testing.T) {
	d := NewDecoder(builder.DecoderConfig{})
	if d.maxBody != gconst.DefaultHandlerBodySize {
		t.Fatalf("maxBody = %d, want %d", d.maxBody, gconst.DefaultHandlerBodySize)
	}
}

// TestDecoderNoBody 无请求体的请求（GET）Body 应为空。
func TestDecoderNoBody(t *testing.T) {
	d := NewDecoder(builder.DecoderConfig{MaxBody: 4096})
	req := readTestRequest(t, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")

	got, err := d.Decode(req)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Body) != 0 {
		t.Errorf("Body = %q, want empty", got.Body)
	}
	if got.Method != "GET" {
		t.Errorf("Method = %q, want GET", got.Method)
	}
}

// TestDecoderBodyAtMaxSize body 长度恰好等于 maxBody 时应正常通过。
func TestDecoderBodyAtMaxSize(t *testing.T) {
	d := NewDecoder(builder.DecoderConfig{MaxBody: 5})
	req := readTestRequest(t, "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhello")

	got, err := d.Decode(req)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got.Body) != "hello" {
		t.Errorf("Body = %q, want hello", got.Body)
	}
}

// TestDecoderBodyOneByteOverMaxSize ContentLength 比 maxBody 大 1 时触发 ErrBodyTooLarge。
func TestDecoderBodyOneByteOverMaxSize(t *testing.T) {
	d := NewDecoder(builder.DecoderConfig{MaxBody: 4})
	req := readTestRequest(t, "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhello")

	_, err := d.Decode(req)
	if !errors.Is(err, gerrors.ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
}

// TestDecoderChunkedBodyTooLarge chunked 请求的实际 body 超过 maxBody 时触发 ErrBodyTooLarge。
func TestDecoderChunkedBodyTooLarge(t *testing.T) {
	d := NewDecoder(builder.DecoderConfig{MaxBody: 4})
	raw := "POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n" +
		"5\r\nhello\r\n" +
		"1\r\n!\r\n" +
		"0\r\n\r\n"
	req := readTestRequest(t, raw)
	if req.ContentLength != -1 {
		t.Fatalf("ContentLength = %d, want -1 for chunked", req.ContentLength)
	}

	_, err := d.Decode(req)
	if !errors.Is(err, gerrors.ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
}

// TestDecoderBodyShorterThanContentLength 实际 body 字节数少于 Content-Length 声明时返回 ErrDecodeRequest。
func TestDecoderBodyShorterThanContentLength(t *testing.T) {
	d := NewDecoder(builder.DecoderConfig{MaxBody: 20})
	req := readTestRequest(t, "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 10\r\n\r\nhello")

	_, err := d.Decode(req)
	if !errors.Is(err, gerrors.ErrDecodeRequest) {
		t.Fatalf("err = %v, want ErrDecodeRequest", err)
	}
}


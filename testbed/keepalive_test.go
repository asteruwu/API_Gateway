package testbed

import (
	"API_Gateway/builder"
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// echoViaKeepAlive 在同一条连接上发送一个请求并读回响应体，不关闭连接。
// closeAfter 为 true 时在请求头追加 Connection: close，期望服务端处理后关闭连接。
func echoViaKeepAlive(t *testing.T, conn net.Conn, reader *bufio.Reader, want []byte, closeAfter bool) []byte {
	t.Helper()
	connHeader := ""
	if closeAfter {
		connHeader = "Connection: close\r\n"
	}
	req := fmt.Sprintf("POST / HTTP/1.1\r\nHost: gateway\r\n%sContent-Length: %d\r\n\r\n%s",
		connHeader, len(want), want)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	got, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return got
}

// TestEchoKeepAlive 验证同一条连接上连续发送多个请求均能正确 echo（不粘连），
// 且最后一个带 Connection: close 的请求会触发服务端主动关闭连接。
func TestEchoKeepAlive(t *testing.T) {
	backendAddr := startEchoBackend(t)

	cfg := &builder.Config{
		Connector: builder.ConnectorConfig{Port: "9997"},
		Backend: builder.BackendConfig{
			Service: []builder.ServiceConfig{
				{Name: "testBackend", Instances: []builder.InstanceConfig{{Addr: backendAddr}}},
			},
		},
	}
	startGateway(t, cfg)

	conn, err := net.Dial("tcp", "127.0.0.1:9997")
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	wants := [][]byte{[]byte("first"), []byte("second"), []byte("third")}
	for i, want := range wants {
		closeAfter := i == len(wants)-1
		got := echoViaKeepAlive(t, conn, reader, want, closeAfter)
		if string(got) != string(want) {
			t.Errorf("request %d echo mismatch: got %q, want %q", i, got, want)
		}
	}

	// 最后一个请求带 Connection: close，服务端应主动关闭连接，读应得到 EOF
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := reader.Read(make([]byte, 1)); err != io.EOF {
		t.Errorf("expected EOF after Connection: close, got %v", err)
	}
}

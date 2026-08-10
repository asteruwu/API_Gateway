package testbed

import (
	"API_Gateway/backend"
	"API_Gateway/builder"
	"API_Gateway/connector"
	"API_Gateway/handler"
	"API_Gateway/testbed/fakebackend"
	"io"
	"net"
	"testing"
	"time"
)

func TestEchoEndToEnd(t *testing.T) {
	// 1. 起假后端
	fb := &fakebackend.FakeBackend{
		Addr: "127.0.0.1:0",
		Mode: fakebackend.ModeEcho,
	}
	if err := fb.Start(); err != nil {
		t.Fatalf("fakebackend start: %v", err)
	}
	defer fb.Close()
	backendAddr := fb.RealAddr()

	// 2. 用假后端真实地址构造配置
	cfg := &builder.Config{
		Connector: builder.ConnectorConfig{
			Port: "9999",
		},
		Backend: builder.BackendConfig{
			Service: []builder.ServiceConfig{
				{
					Name: "testBackend",
					Instances: []builder.InstanceConfig{
						{Addr: backendAddr},
					},
				},
			},
		},
	}

	// 3. 组装网关
	bm := backend.NewBManager(cfg.Backend)
	hdl := handler.NewHandler(cfg.Handler, bm.Call)
	lst := connector.NewListener(cfg.Connector, hdl.HandleHTTPConn)

	// 4. 后台跑 Accept 循环
	go func() {
		if err := lst.Connect(); err != nil {
			t.Logf("listener stopped: %v", err)
		}
	}()
	defer lst.Close()

	// 5. ready
	time.Sleep(100 * time.Millisecond)

	// 6. 客户端：half-close 节奏发请求、读响应
	conn, err := net.Dial("tcp", "127.0.0.1:9999")
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer conn.Close()

	want := []byte("hello")
	if _, err := conn.Write(want); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.CloseWrite()
	}

	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("echo mismatch: got %q, want %q", got, want)
	}
}

package testbed

import (
	"API_Gateway/builder"
	"API_Gateway/testbed/fakebackend"
	"bufio"
	"fmt"
	"net"
	"testing"
)

// TestBackendConnReuse 验证帧定界后的连接复用：同一网关连接上连续发送多个请求，
// 池上限锁死为 1 条，若连接不可复用则第 2 个请求将无连接可用；最终后端只应被建立 1 条连接。
func TestBackendConnReuse(t *testing.T) {
	fb := &fakebackend.FakeBackend{Addr: "127.0.0.1:0", Mode: fakebackend.ModeEcho}
	if err := fb.Start(); err != nil {
		t.Fatalf("fakebackend start: %v", err)
	}
	t.Cleanup(func() { _ = fb.Close() })

	const gatewayAddr = "127.0.0.1:9994"
	cfg := &builder.Config{
		Connector: builder.ConnectorConfig{Port: "9994"},
		Handler: builder.HandlerConfig{
			Filter: builder.HTTPFilterConfig{
				Filters: []any{routerConfig(nil, map[string][]string{"testBackend": {"/"}})},
			},
		},
		Backend: backendConfig(map[string][]string{"testBackend": {fb.RealAddr()}}),
	}
	// 单连接上限：强制复用，MaxConn 之外的请求会等待而非新建
	cfg.Backend.Pool = builder.PoolConfig{MaxConn: 1}
	startGateway(t, cfg)

	conn, err := net.Dial("tcp", gatewayAddr)
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for i := 0; i < 3; i++ {
		want := fmt.Appendf([]byte{}, "msg-%d", i)
		got := echoViaKeepAlive(t, conn, reader, want, false)
		if string(got) != string(want) {
			t.Fatalf("request %d: got %q, want %q", i, got, want)
		}
	}

	if n := fb.ConnsAccepted(); n != 1 {
		t.Errorf("backend accepted %d connections, want 1 (connection not reused)", n)
	}
}

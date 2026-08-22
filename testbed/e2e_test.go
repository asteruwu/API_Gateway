package testbed

import (
	"API_Gateway/builder"
	"io"
	"net"
	"testing"
	"time"
)

// TestEchoConcurrentLimit 连接级限流：maxConn=2 时，并发第 3 条连接被拒，
// 前两条正常 echo；连接关闭后再建立的连接应重新获得名额。
func TestEchoConcurrentLimit(t *testing.T) {
	backendAddr := startEchoBackend(t)

	// 配置：连接级限流上限 2，独立端口避免与既有用例冲突
	const gatewayAddr = "127.0.0.1:9998"
	cfg := &builder.Config{
		Connector: builder.ConnectorConfig{
			Port: "9998",
			Filters: []any{
				builder.TCPLimiterFilterConfig{Enable: true, MaxConn: 2},
			},
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
	startGateway(t, cfg)

	// 并发建立 3 条连接，前两条通过 filter 后阻塞在读请求上占住名额；第 3 条必被限流拒绝。
	conns := make([]net.Conn, 0, 3)
	for i := 0; i < 3; i++ {
		conn, err := net.Dial("tcp", gatewayAddr)
		if err != nil {
			t.Fatalf("dial gateway #%d: %v", i, err)
		}
		defer conn.Close()
		conns = append(conns, conn)
	}

	// 第 3 条连接应被拒绝：连接被网关立即关闭，读到空（EOF），不能有响应内容
	_ = conns[2].SetReadDeadline(time.Now().Add(2 * time.Second))
	gotRejected, err := io.ReadAll(conns[2])
	if err != nil {
		t.Fatalf("conn3 read should return EOF from closure, got err: %v", err)
	}
	if len(gotRejected) != 0 {
		t.Errorf("conn3 got response %q, want empty (should be rejected)", gotRejected)
	}

	// 前两条连接正常请求，应完整 echo
	want := []byte("hello")
	for i := 0; i < 2; i++ {
		got := echoViaHalfClose(t, conns[i], want)
		if string(got) != string(want) {
			t.Errorf("conn%d echo mismatch: got %q, want %q", i, got, want)
		}
	}

	// 关闭前两条连接（归还计数），新连接应重新获得名额
	_ = conns[0].Close()
	_ = conns[1].Close()
	time.Sleep(50 * time.Millisecond)

	connN, err := net.Dial("tcp", gatewayAddr)
	if err != nil {
		t.Fatalf("dial after release: %v", err)
	}
	defer connN.Close()
	if got := echoViaHalfClose(t, connN, want); string(got) != string(want) {
		t.Errorf("echo mismatch after release: got %q, want %q", got, want)
	}
}

// TestEchoEndToEnd 端到端打通：客户端 → 网关 → 假后端 → 客户端，返回 echo 数据。
func TestEchoEndToEnd(t *testing.T) {
	backendAddr := startEchoBackend(t)

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
	startGateway(t, cfg)

	// 客户端：half-close 节奏发请求、读响应
	conn, err := net.Dial("tcp", "127.0.0.1:9999")
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer conn.Close()

	want := []byte("hello")
	got := echoViaHalfClose(t, conn, want)
	if string(got) != string(want) {
		t.Errorf("echo mismatch: got %q, want %q", got, want)
	}
}

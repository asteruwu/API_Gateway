package testbed

import (
	"API_Gateway/backend"
	"API_Gateway/builder"
	"API_Gateway/connector"
	"API_Gateway/handler"
	"API_Gateway/testbed/fakebackend"
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// startEchoBackend 启动一个 echo 模式假后端，返回其监听地址。
// 测试结束时由 t.Cleanup 统一关闭，测试内无需手动 Close。
func startEchoBackend(t *testing.T) string {
	t.Helper()
	fb := &fakebackend.FakeBackend{
		Addr: "127.0.0.1:0",
		Mode: fakebackend.ModeEcho,
	}
	if err := fb.Start(); err != nil {
		t.Fatalf("fakebackend start: %v", err)
	}
	t.Cleanup(func() { _ = fb.Close() })
	return fb.RealAddr()
}

// startGateway 用配置组装完整网关（backend → handler → connector 逆序接线），
// 后台启动 Accept 循环并等待就绪。测试结束时由 t.Cleanup 关闭监听器。
func startGateway(t *testing.T, cfg *builder.Config) {
	t.Helper()
	bm := backend.NewBManager(cfg.Backend)
	hdl, err := handler.NewHandler(cfg.Handler, bm.Call)
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	lst, err := connector.NewListener(cfg.Connector, hdl.HandleHTTPConn)
	if err != nil {
		t.Fatalf("build listener: %v", err)
	}
	go func() {
		if err := lst.Connect(); err != nil {
			t.Logf("listener stopped: %v", err)
		}
	}()
	t.Cleanup(lst.Close)
	// 等待 Accept 循环就绪
	time.Sleep(100 * time.Millisecond)
}

// echoViaHalfClose 向 conn 发送一个携带 want 作为请求体的合法 HTTP 请求，
// 然后 half-close（网关 handler 因此读到 EOF 结束本次连接生命周期），读回响应。
func echoViaHalfClose(t *testing.T, conn net.Conn, want []byte) []byte {
	t.Helper()
	req := fmt.Sprintf("POST / HTTP/1.1\r\nHost: gateway\r\nContent-Length: %d\r\n\r\n%s",
		len(want), want)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.CloseWrite()
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return got
}

// routerConfig 按「服务名 → 路径前缀列表」构造路由 filter 配置
func routerConfig(rules map[string][]string) builder.RouterConfig {
	groups := make([]builder.RouteServiceConfig, 0, len(rules))
	for service, prefixes := range rules {
		rs := make([]builder.RouteRuleConfig, 0, len(prefixes))
		for _, p := range prefixes {
			rs = append(rs, builder.RouteRuleConfig{PathPrefix: p})
		}
		groups = append(groups, builder.RouteServiceConfig{Service: service, Rules: rs})
	}
	return builder.RouterConfig{Router: groups}
}

// backendConfig 按「服务名 → 实例地址列表」构造 backend 配置
func backendConfig(services map[string][]string) builder.BackendConfig {
	svcList := make([]builder.ServiceConfig, 0, len(services))
	for name, addrs := range services {
		instances := make([]builder.InstanceConfig, 0, len(addrs))
		for _, addr := range addrs {
			instances = append(instances, builder.InstanceConfig{Addr: addr})
		}
		svcList = append(svcList, builder.ServiceConfig{
			Name:      name,
			Instances: instances,
		})
	}
	return builder.BackendConfig{Service: svcList}
}

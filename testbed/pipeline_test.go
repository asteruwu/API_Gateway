package testbed

import (
	"bufio"
	"net"
	"net/http"
	"testing"

	"API_Gateway/builder"
	"API_Gateway/testbed/fakebackend"
)

// TestDefaultPipelineEquivalence C4：route 不声明 filters 时继承 http_filters.enabled，
// 显式空链不执行任何 filter；默认链场景应与 v07 的「Router → enabled」顺序一致。
func TestDefaultPipelineEquivalence(t *testing.T) {
	t.Setenv(authTestSecretEnv, string(authTestSecret))

	fb := &fakebackend.FakeBackend{
		Addr:      "127.0.0.1:0",
		Mode:      fakebackend.ModeFixed,
		FixedResp: []byte("resp-from-backend"),
	}
	if err := fb.Start(); err != nil {
		t.Fatalf("fakebackend start: %v", err)
	}
	t.Cleanup(func() { _ = fb.Close() })

	const gatewayAddr = "127.0.0.1:9992"
	router := builder.RouterConfig{Rules: []builder.RouterRule{
		{ID: "default#0", PathPrefix: "/default", Service: "svc"},
		{ID: "open#0", PathPrefix: "/open", Service: "svc"},
	}}
	cfg := &builder.Config{
		Connector: builder.ConnectorConfig{Port: "9992"},
		Handler: builder.HandlerConfig{
			Filter: builder.HTTPFilterConfig{
				Router: router,
				Configs: map[string]any{
					"auth": builder.AuthFilterConfig{
						Credentials: []string{"bearer"},
						JWT:         builder.JWTVerifierConfig{SecretEnv: authTestSecretEnv},
						Policies: map[string]builder.AuthPolicy{
							"default#0": {Credential: "bearer"},
						},
					},
				},
				// 等价于 Compile 的产物：
				// /default 未声明 filters → 继承 enabled=[auth]；/open 显式 [] → 空链
				Pipelines: map[string][]string{
					"default#0": {"auth"},
					"open#0":    {},
				},
			},
		},
		Backend: backendConfig(map[string][]string{"svc": {fb.RealAddr()}}),
	}
	startGateway(t, cfg)

	conn, err := net.Dial("tcp", gatewayAddr)
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// 默认链：/default 未带凭证 → auth 执行并拒绝 401
	status, header, _ := doAuthRequest(t, conn, reader, "/default", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("/default: status = %d, want 401 (default chain must run auth)", status)
	}
	if header.Get("WWW-Authenticate") != "Bearer" {
		t.Errorf("/default: WWW-Authenticate = %q, want Bearer", header.Get("WWW-Authenticate"))
	}
	// 被认证拒绝的请求不得触达后端
	if n := fb.ConnsAccepted(); n != 0 {
		t.Fatalf("backend accepted %d connections after rejected request, want 0", n)
	}

	// 显式空链：/open 不执行 auth，直接到后端
	status, _, body := doAuthRequest(t, conn, reader, "/open", "")
	if status != http.StatusOK || string(body) != "resp-from-backend" {
		t.Fatalf("/open: status = %d body = %q, want 200 resp-from-backend", status, body)
	}
	if n := fb.ConnsAccepted(); n == 0 {
		t.Error("backend accepted 0 connections after /open, want >= 1")
	}
}

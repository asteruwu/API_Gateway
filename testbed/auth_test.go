package testbed

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"API_Gateway/builder"
	"API_Gateway/testbed/fakebackend"
)

const authTestSecretEnv = "GATEWAY_TEST_JWT_SECRET"

// authTestSecret 32 字节，满足 HS256 密钥最短长度要求
var authTestSecret = []byte("0123456789abcdef0123456789abcdef")

// signTestJWT 用给定密钥对 header/claims 做 HS256 签名，产出合法 token
func signTestJWT(t *testing.T, secret []byte, claims map[string]any) string {
	t.Helper()
	hb, err := json.Marshal(map[string]any{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// testClaims 构造带 sub/exp 的 claim 集，roles 非 nil 时写入角色
func testClaims(roles []string, exp time.Time) map[string]any {
	c := map[string]any{"sub": "user-1", "exp": float64(exp.Unix())}
	if roles != nil {
		c["roles"] = roles
	}
	return c
}

// doAuthRequest 在同一条 keep-alive 连接上发送带可选 Bearer 凭证的请求，
// 读回状态码、响应头与响应体。
func doAuthRequest(t *testing.T, conn net.Conn, reader *bufio.Reader, path, token string) (int, http.Header, []byte) {
	t.Helper()
	authHeader := ""
	if token != "" {
		authHeader = "Authorization: Bearer " + token + "\r\n"
	}
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: gateway\r\n%sContent-Length: 0\r\n\r\n", path, authHeader)
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp.StatusCode, resp.Header, body
}

// TestAuthEndToEnd 端到端鉴权：合法 200 / 缺失或非法凭证 401 / 角色不足 403 /
// public 免鉴权；拒绝路径不得触达后端（后端 accept 数保持 0）。
func TestAuthEndToEnd(t *testing.T) {
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

	// Router 在前写入 RouteID，AuthFilter 按 RouteID 查策略（顺序不可颠倒）
	const gatewayAddr = "127.0.0.1:9993"
	cfg := &builder.Config{
		Connector: builder.ConnectorConfig{Port: "9993"},
		Handler: builder.HandlerConfig{
			Filter: builder.HTTPFilterConfig{Filters: []any{
				builder.RouterConfig{Rules: []builder.RouterRule{
					{ID: "public#0", PathPrefix: "/public", Service: "svc"},
					{ID: "orders#0", PathPrefix: "/orders", Service: "svc"},
					{ID: "admin#0", PathPrefix: "/admin", Service: "svc"},
				}},
				builder.AuthFilterConfig{
					Credentials: []string{"bearer"},
					JWT:         builder.JWTVerifierConfig{SecretEnv: authTestSecretEnv},
					Policies: map[string]builder.AuthPolicy{
						"public#0": {Public: true},
						"orders#0": {Credential: "bearer"},
						"admin#0":  {Credential: "bearer", Roles: []string{"admin"}},
					},
				},
			}},
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

	now := time.Now()
	validUser := signTestJWT(t, authTestSecret, testClaims([]string{"user"}, now.Add(time.Hour)))
	validAdmin := signTestJWT(t, authTestSecret, testClaims([]string{"admin"}, now.Add(time.Hour)))
	expired := signTestJWT(t, authTestSecret, testClaims([]string{"admin"}, now.Add(-time.Hour)))

	// —— 拒绝路径 ——
	rejected := []struct {
		name       string
		path       string
		token      string
		wantStatus int
	}{
		{"missing credential", "/orders", "", http.StatusUnauthorized},
		{"malformed token", "/orders", "not-a-jwt", http.StatusUnauthorized},
		{"expired token", "/orders", expired, http.StatusUnauthorized},
		{"insufficient role", "/admin", validUser, http.StatusForbidden},
	}
	for _, c := range rejected {
		status, header, body := doAuthRequest(t, conn, reader, c.path, c.token)
		if status != c.wantStatus {
			t.Errorf("%s: status = %d body = %q, want %d", c.name, status, body, c.wantStatus)
		}
		if c.wantStatus == http.StatusUnauthorized && header.Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("%s: WWW-Authenticate = %q, want Bearer", c.name, header.Get("WWW-Authenticate"))
		}
	}

	// 拒绝必须发生在借出后端连接之前
	if n := fb.ConnsAccepted(); n != 0 {
		t.Errorf("backend accepted %d connections during rejected requests, want 0", n)
	}

	// —— 放行路径：public 免鉴权 + 认证/授权通过 ——
	allowed := []struct {
		name  string
		path  string
		token string
	}{
		{"public", "/public", ""},
		{"authenticated", "/orders", validUser},
		{"authorized", "/admin", validAdmin},
	}
	for _, c := range allowed {
		status, _, body := doAuthRequest(t, conn, reader, c.path, c.token)
		if status != http.StatusOK || string(body) != "resp-from-backend" {
			t.Errorf("%s: status = %d body = %q, want 200 resp-from-backend", c.name, status, body)
		}
	}

	if n := fb.ConnsAccepted(); n == 0 {
		t.Error("backend accepted 0 connections after successful requests, want >= 1")
	}
}

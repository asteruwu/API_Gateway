package auth

import (
	"errors"
	"net/http"
	"slices"
	"testing"

	"API_Gateway/builder"
	"API_Gateway/handler/http_filter/auth/verifier"
	"API_Gateway/handler/message"
	gerrors "API_Gateway/pkg/errors"
)

// stubVerifier 记录调用并返回预置结果
type stubVerifier struct {
	result *verifier.Verified
	err    error
	called bool
	got    string
}

func (s *stubVerifier) Verify(credential string) (*verifier.Verified, error) {
	s.called = true
	s.got = credential
	return s.result, s.err
}

// bearerRequest 构造带/不带 Bearer 凭证的请求
func bearerRequest(routeID, credential string) *message.Request {
	h := http.Header{}
	if credential != "" {
		h.Set("Authorization", "Bearer "+credential)
	}
	return &message.Request{RouteID: routeID, Header: h}
}

// testFilter 构造单条策略（RouteID = shopA.order#0）的 filter
func testFilter(v verifier.Verifier, policy builder.AuthPolicy) *AuthFilter {
	return &AuthFilter{
		verifier: v,
		policies: map[string]builder.AuthPolicy{"shopA.order#0": policy},
	}
}

// TestAuthFilterPublic public 路由直接放行，不调用 verifier，也不写 Identity
func TestAuthFilterPublic(t *testing.T) {
	v := &stubVerifier{}
	f := testFilter(v, builder.AuthPolicy{Public: true})
	req := bearerRequest("shopA.order#0", "") // 无凭证也应放行

	resp, err := f.HandleHTTPFilt(req)
	if err != nil || resp != nil {
		t.Fatalf("resp = %v, err = %v, want pass-through", resp, err)
	}
	if v.called {
		t.Error("verifier must not be called for public route")
	}
	if req.Identity != nil {
		t.Errorf("Identity = %+v, want nil for public route", req.Identity)
	}
}

// TestAuthFilterSuccess 凭证合法且满足角色/权限：回写 Identity 后放行
func TestAuthFilterSuccess(t *testing.T) {
	v := &stubVerifier{result: &verifier.Verified{Subject: "user-1", Roles: []string{"admin"}, Scopes: []string{"orders:read"}}}
	f := testFilter(v, builder.AuthPolicy{Credential: "bearer", Roles: []string{"admin"}, Scopes: []string{"orders:read"}})
	req := bearerRequest("shopA.order#0", "jwt-token")

	resp, err := f.HandleHTTPFilt(req)
	if err != nil || resp != nil {
		t.Fatalf("resp = %v, err = %v, want pass-through", resp, err)
	}
	if v.got != "jwt-token" {
		t.Errorf("verifier got %q, want jwt-token", v.got)
	}
	if req.Identity == nil || req.Identity.Subject != "user-1" || !slices.Equal(req.Identity.Roles, []string{"admin"}) {
		t.Errorf("Identity = %+v, want subject user-1 roles [admin]", req.Identity)
	}
}

// TestAuthFilterMissingCredential 缺少凭证：ErrUnauthorized，且不调用 verifier
func TestAuthFilterMissingCredential(t *testing.T) {
	v := &stubVerifier{}
	f := testFilter(v, builder.AuthPolicy{Credential: "bearer"})
	req := bearerRequest("shopA.order#0", "")

	if _, err := f.HandleHTTPFilt(req); !errors.Is(err, gerrors.ErrUnauthorized) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrUnauthorized)
	}
	if v.called {
		t.Error("verifier must not be called without credential")
	}
}

// TestAuthFilterVerifyFailed 认证失败：ErrUnauthorized；空 Roles/Scopes 也必须先验签（防绕过）
func TestAuthFilterVerifyFailed(t *testing.T) {
	v := &stubVerifier{err: gerrors.ErrUnauthorized}
	f := testFilter(v, builder.AuthPolicy{Credential: "bearer"})
	req := bearerRequest("shopA.order#0", "bad")

	if _, err := f.HandleHTTPFilt(req); !errors.Is(err, gerrors.ErrUnauthorized) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrUnauthorized)
	}
	if !v.called {
		t.Error("verifier must be called even when no authorization is required")
	}
}

// TestAuthFilterRoleDenied 角色不足：拒绝，且不回写 Identity
func TestAuthFilterRoleDenied(t *testing.T) {
	v := &stubVerifier{result: &verifier.Verified{Subject: "u", Roles: []string{"user"}}}
	f := testFilter(v, builder.AuthPolicy{Credential: "bearer", Roles: []string{"admin"}})
	req := bearerRequest("shopA.order#0", "token")

	if _, err := f.HandleHTTPFilt(req); !errors.Is(err, gerrors.ErrForbidden) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrForbidden)
	}
	if req.Identity != nil {
		t.Error("Identity must not be written when authorization fails")
	}
}

// TestAuthFilterScopeDenied 权限不足：拒绝
func TestAuthFilterScopeDenied(t *testing.T) {
	v := &stubVerifier{result: &verifier.Verified{Subject: "u", Scopes: []string{"orders:read"}}}
	f := testFilter(v, builder.AuthPolicy{Credential: "bearer", Scopes: []string{"orders:write"}})
	req := bearerRequest("shopA.order#0", "token")

	if _, err := f.HandleHTTPFilt(req); !errors.Is(err, gerrors.ErrForbidden) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrForbidden)
	}
}

// TestAuthFilterAuthOnly 空 Roles/Scopes：只认证不授权，验签通过即放行
func TestAuthFilterAuthOnly(t *testing.T) {
	v := &stubVerifier{result: &verifier.Verified{Subject: "u"}}
	f := testFilter(v, builder.AuthPolicy{Credential: "bearer"})
	req := bearerRequest("shopA.order#0", "token")

	if _, err := f.HandleHTTPFilt(req); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if req.Identity == nil || req.Identity.Subject != "u" {
		t.Errorf("Identity = %+v, want subject u", req.Identity)
	}
}

// TestAuthFilterNoPolicy 命中路由但策略表缺失：fail-closed，且不调用 verifier
func TestAuthFilterNoPolicy(t *testing.T) {
	v := &stubVerifier{}
	f := &AuthFilter{verifier: v, policies: map[string]builder.AuthPolicy{}}
	req := bearerRequest("shopA.order#0", "token")

	if _, err := f.HandleHTTPFilt(req); !errors.Is(err, gerrors.ErrForbidden) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrForbidden)
	}
	if v.called {
		t.Error("verifier must not be called when no policy exists")
	}
}

// TestBearerToken Authorization 头解析：大小写不敏感、容忍多余空格、非法形态拒绝
func TestBearerToken(t *testing.T) {
	cases := map[string]struct {
		header string
		want   string
		ok     bool
	}{
		"plain":        {"Bearer abc", "abc", true},
		"lowercase":    {"bearer abc", "abc", true},
		"extra spaces": {"Bearer   abc  ", "abc", true},
		"empty":        {"", "", false},
		"prefix only":  {"Bearer ", "", false},
		"wrong scheme": {"Basic abc", "", false},
	}
	for name, c := range cases {
		got, ok := bearerToken(c.header)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: bearerToken(%q) = (%q, %v), want (%q, %v)", name, c.header, got, ok, c.want, c.ok)
		}
	}
}

// TestNewAuthFilterRequiresSecret 构造时密钥缺失应报错
func TestNewAuthFilterRequiresSecret(t *testing.T) {
	t.Setenv("AUTH_TEST_SECRET", "")
	_, err := NewAuthFilter(builder.AuthFilterConfig{
		JWT: builder.JWTVerifierConfig{SecretEnv: "AUTH_TEST_SECRET"},
	})
	if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
	}
}

// TestAuthFilterStripsReservedHeaders 入站网关保留命名空间（X-Gateway-*）下的头必须被剥离（public 与鉴权路由都剥）
func TestAuthFilterStripsReservedHeaders(t *testing.T) {
	for name, policy := range map[string]builder.AuthPolicy{
		"public":        {Public: true},
		"authenticated": {Credential: "bearer"},
	} {
		t.Run(name, func(t *testing.T) {
			v := &stubVerifier{result: &verifier.Verified{Subject: "u"}}
			f := testFilter(v, policy)
			req := bearerRequest("shopA.order#0", "token")
			req.Header.Set("X-Gateway-User-Id", "forged-admin")
			req.Header.Set("X-Gateway-Roles", "admin")
			req.Header.Set("X-User-Id", "business") // 非保留命名空间，应保留
			req.Header.Set("X-Keep", "ok")

			if _, err := f.HandleHTTPFilt(req); err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got := req.Header.Get("X-Gateway-User-Id"); got != "" {
				t.Errorf("X-Gateway-User-Id = %q, want stripped", got)
			}
			if got := req.Header.Get("X-Gateway-Roles"); got != "" {
				t.Errorf("X-Gateway-Roles = %q, want stripped", got)
			}
			if got := req.Header.Get("X-User-Id"); got != "business" {
				t.Errorf("X-User-Id = %q, want kept（非保留命名空间）", got)
			}
			if got := req.Header.Get("X-Keep"); got != "ok" {
				t.Errorf("X-Keep = %q, want untouched", got)
			}
		})
	}
}

// TestStripReservedHeadersCaseInsensitive 非规范大小写的保留头也要被剥离
func TestStripReservedHeadersCaseInsensitive(t *testing.T) {
	h := http.Header{}
	h["x-gateway-user-id"] = []string{"forged"} // 直接写入非规范 key
	h["X-Gateway-Roles"] = []string{"admin"}
	h.Set("X-Keep", "ok")

	stripReservedHeaders(h)

	if len(h) != 1 || h.Get("X-Keep") != "ok" {
		t.Errorf("header = %v, want only X-Keep", h)
	}
}

func TestAuthFilterUnauthorizedCarriesChallenge(t *testing.T) {
	cases := map[string]struct {
		credential string
		token      string
		verifier   *stubVerifier
		wantCalled bool
	}{
		"missing credential":     {credential: "bearer", verifier: &stubVerifier{}},
		"invalid credential":     {credential: "bearer", token: "bad", verifier: &stubVerifier{err: gerrors.ErrUnauthorized}, wantCalled: true},
		"unsupported credential": {credential: "basic", token: "x", verifier: &stubVerifier{}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := testFilter(c.verifier, builder.AuthPolicy{Credential: c.credential})
			req := bearerRequest("shopA.order#0", c.token)

			resp, err := f.HandleHTTPFilt(req)
			if !errors.Is(err, gerrors.ErrUnauthorized) {
				t.Fatalf("err = %v, want %v", err, gerrors.ErrUnauthorized)
			}
			if resp == nil {
				t.Fatal("resp = nil, want partial response carrying the challenge")
			}
			if got := resp.Header.Get("WWW-Authenticate"); got != "Bearer" {
				t.Errorf("WWW-Authenticate = %q, want Bearer", got)
			}
			if resp.StatusCode != 0 || len(resp.Body) != 0 {
				t.Errorf("partial response must not set status/body, got status=%d body=%q", resp.StatusCode, resp.Body)
			}
			if c.verifier.called != c.wantCalled {
				t.Errorf("verifier called = %v, want %v", c.verifier.called, c.wantCalled)
			}
		})
	}
}

func TestAuthFilterForbiddenCarriesNoResponse(t *testing.T) {
	cases := map[string]*AuthFilter{
		"role": {
			verifier: &stubVerifier{result: &verifier.Verified{Subject: "u", Roles: []string{"user"}}},
			policies: map[string]builder.AuthPolicy{
				"shopA.order#0": {Credential: "bearer", Roles: []string{"admin"}},
			},
		},
		"scope": {
			verifier: &stubVerifier{result: &verifier.Verified{Subject: "u", Scopes: []string{"orders:read"}}},
			policies: map[string]builder.AuthPolicy{
				"shopA.order#0": {Credential: "bearer", Scopes: []string{"orders:write"}},
			},
		},
		"no policy": {
			verifier: &stubVerifier{},
			policies: map[string]builder.AuthPolicy{},
		},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			resp, err := f.HandleHTTPFilt(bearerRequest("shopA.order#0", "token"))
			if !errors.Is(err, gerrors.ErrForbidden) {
				t.Fatalf("err = %v, want %v", err, gerrors.ErrForbidden)
			}
			if resp != nil {
				t.Errorf("resp = %+v, want nil for forbidden", resp)
			}
		})
	}
}

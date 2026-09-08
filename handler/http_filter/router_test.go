package httpfilter

import (
	"errors"
	"net/url"
	"testing"

	"API_Gateway/builder"
	"API_Gateway/handler/message"
	gerrors "API_Gateway/pkg/errors"
)

func testRequest(method, path string) *message.Request {
	return &message.Request{
		Method: method,
		URL:    &url.URL{Path: path},
	}
}

func testRouter(t *testing.T) *Router {
	t.Helper()
	r, err := NewRouter(builder.RouterConfig{
		Router: []builder.RouteServiceConfig{
			{Service: "svcRefund", Rules: []builder.RouteRuleConfig{{PathPrefix: "/orders/refund"}}},
			{Service: "svcOrder", Rules: []builder.RouteRuleConfig{{PathPrefix: "/orders"}}},
			{Service: "svcWeb", Rules: []builder.RouteRuleConfig{{PathPrefix: "/"}}},
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return r
}

// TestRouterLongestPrefix 多条候选时取前缀最长者，兜底规则接住其余路径
func TestRouterLongestPrefix(t *testing.T) {
	r := testRouter(t)

	cases := []struct {
		path string
		want string
	}{
		{"/orders/refund/123", "svcRefund"},
		{"/orders/refund", "svcRefund"}, // 精确匹配：前缀恰好等于完整路径
		{"/orders/abc", "svcOrder"},
		{"/other", "svcWeb"}, // 兜底
	}

	for _, c := range cases {
		req := testRequest("GET", c.path)
		if _, err := r.HandleHTTPFilt(req); err != nil {
			t.Errorf("%s: unexpected err: %v", c.path, err)
			continue
		}
		if req.Service != c.want {
			t.Errorf("%s: service = %q, want %q", c.path, req.Service, c.want)
		}
	}
}

// TestRouterNotFound 无兜底规则时未命中返回 ErrRouteNotFound
func TestRouterNotFound(t *testing.T) {
	r, err := NewRouter(builder.RouterConfig{
		Router: []builder.RouteServiceConfig{
			{Service: "svcOrder", Rules: []builder.RouteRuleConfig{{PathPrefix: "/orders"}}},
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	req := testRequest("GET", "/users")
	_, err = r.HandleHTTPFilt(req)
	if !errors.Is(err, gerrors.ErrRouteNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrRouteNotFound)
	}
}

// TestRouterMethodGuard 方法守卫：方法不符时跳过该规则、回落到更短规则
func TestRouterMethodGuard(t *testing.T) {
	r, err := NewRouter(builder.RouterConfig{
		Router: []builder.RouteServiceConfig{
			{Service: "svcPost", Rules: []builder.RouteRuleConfig{{PathPrefix: "/orders", Methods: []string{"POST"}}}},
			{Service: "svcWeb", Rules: []builder.RouteRuleConfig{{PathPrefix: "/"}}},
		},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	req := testRequest("POST", "/orders/1")
	if _, err := r.HandleHTTPFilt(req); err != nil {
		t.Fatalf("POST /orders/1 unexpected err: %v", err)
	}
	if req.Service != "svcPost" {
		t.Errorf("POST service = %q, want svcPost", req.Service)
	}

	req = testRequest("GET", "/orders/1")
	if _, err := r.HandleHTTPFilt(req); err != nil {
		t.Fatalf("GET /orders/1 unexpected err: %v", err)
	}
	if req.Service != "svcWeb" {
		t.Errorf("GET service = %q, want svcWeb（方法不符回落到更短规则）", req.Service)
	}
}

// TestRouterRejectInvalidConfig 非法配置在加载期拒绝
func TestRouterRejectInvalidConfig(t *testing.T) {
	cases := map[string]builder.RouterConfig{
		"empty prefix": {Router: []builder.RouteServiceConfig{
			{Service: "svc", Rules: []builder.RouteRuleConfig{{PathPrefix: ""}}},
		}},
		"duplicate prefix": {Router: []builder.RouteServiceConfig{
			{Service: "svcA", Rules: []builder.RouteRuleConfig{{PathPrefix: "/orders"}}},
			{Service: "svcB", Rules: []builder.RouteRuleConfig{{PathPrefix: "/orders"}}},
		}},
	}
	for name, cfg := range cases {
		if _, err := NewRouter(cfg); !errors.Is(err, gerrors.ErrInitializeHTTPFiltersFailed) {
			t.Errorf("%s: err = %v, want %v", name, err, gerrors.ErrInitializeHTTPFiltersFailed)
		}
	}
}

package httpfilter

import (
	"errors"
	"net/url"
	"testing"

	"API_Gateway/builder"
	"API_Gateway/handler/message"
	gerrors "API_Gateway/pkg/errors"
)

func testRequest(method, host, path string) *message.Request {
	return &message.Request{
		Method: method,
		Host:   host,
		URL:    &url.URL{Path: path},
	}
}

// testRouter 两个平台分组 + 一个通配分组的路由表（编译层展平后的形态）
func testRouter(t *testing.T) *Router {
	t.Helper()
	return NewRouter(builder.RouterConfig{
		Rules: []builder.RouterRule{
			{Hosts: []string{"api.shopA.com"}, PathPrefix: "/orders/refund", Service: "svcRefund"},
			{Hosts: []string{"api.shopA.com"}, PathPrefix: "/orders", Service: "svcOrder"},
			{Hosts: []string{"api.shopB.com"}, PathPrefix: "/orders", Service: "svcShopB"},
			{PathPrefix: "/", Service: "svcWeb"},
		},
	})
}

// TestRouterHostAndPath 域名 + 路径联合匹配：同一路径按域名分发到不同服务，
// 平台内最长前缀优先，域名未命中或其余路径落通配分组
func TestRouterHostAndPath(t *testing.T) {
	r := testRouter(t)

	cases := []struct {
		host string
		path string
		want string
	}{
		{"api.shopA.com", "/orders/refund/123", "svcRefund"},
		{"api.shopA.com", "/orders/refund", "svcRefund"}, // 精确匹配：前缀恰好等于完整路径
		{"api.shopA.com", "/orders/abc", "svcOrder"},
		{"api.shopB.com", "/orders/abc", "svcShopB"}, // 同路径不同域名
		{"api.shopC.com", "/orders/abc", "svcWeb"},   // 域名未命中，落通配分组
		{"api.shopA.com", "/other", "svcWeb"},        // 兜底
	}

	for _, c := range cases {
		req := testRequest("GET", c.host, c.path)
		if _, err := r.HandleHTTPFilt(req); err != nil {
			t.Errorf("%s%s: unexpected err: %v", c.host, c.path, err)
			continue
		}
		if req.Service != c.want {
			t.Errorf("%s%s: service = %q, want %q", c.host, c.path, req.Service, c.want)
		}
	}
}

// TestRouterNotFound 无通配分组时，域名未命中或路径未命中都返回 ErrRouteNotFound
func TestRouterNotFound(t *testing.T) {
	r := NewRouter(builder.RouterConfig{
		Rules: []builder.RouterRule{
			{Hosts: []string{"api.shopA.com"}, PathPrefix: "/orders", Service: "svcOrder"},
		},
	})

	// 域名命中、路径未命中
	req := testRequest("GET", "api.shopA.com", "/users")
	if _, err := r.HandleHTTPFilt(req); !errors.Is(err, gerrors.ErrRouteNotFound) {
		t.Errorf("path miss: err = %v, want %v", err, gerrors.ErrRouteNotFound)
	}

	// 路径命中、域名未命中且无通配分组兜底
	req = testRequest("GET", "api.shopB.com", "/orders")
	if _, err := r.HandleHTTPFilt(req); !errors.Is(err, gerrors.ErrRouteNotFound) {
		t.Errorf("host miss: err = %v, want %v", err, gerrors.ErrRouteNotFound)
	}
}

// TestRouterMethodGuard 方法守卫：方法不符时跳过该规则、回落到更短规则
func TestRouterMethodGuard(t *testing.T) {
	r := NewRouter(builder.RouterConfig{
		Rules: []builder.RouterRule{
			{PathPrefix: "/orders", Methods: []string{"POST"}, Service: "svcPost"},
			{PathPrefix: "/", Service: "svcWeb"},
		},
	})

	req := testRequest("POST", "", "/orders/1")
	if _, err := r.HandleHTTPFilt(req); err != nil {
		t.Fatalf("POST /orders/1 unexpected err: %v", err)
	}
	if req.Service != "svcPost" {
		t.Errorf("POST service = %q, want svcPost", req.Service)
	}

	req = testRequest("GET", "", "/orders/1")
	if _, err := r.HandleHTTPFilt(req); err != nil {
		t.Fatalf("GET /orders/1 unexpected err: %v", err)
	}
	if req.Service != "svcWeb" {
		t.Errorf("GET service = %q, want svcWeb（方法不符回落到更短规则）", req.Service)
	}
}

// TestRouterSpecificHostBeatsWildcard 同前缀时，特定域名规则优先于通配规则
func TestRouterSpecificHostBeatsWildcard(t *testing.T) {
	r := NewRouter(builder.RouterConfig{
		Rules: []builder.RouterRule{
			{PathPrefix: "/orders", Service: "svcDefault"},
			{Hosts: []string{"api.shopA.com"}, PathPrefix: "/orders", Service: "svcShopA"},
		},
	})

	req := testRequest("GET", "api.shopA.com", "/orders")
	if _, err := r.HandleHTTPFilt(req); err != nil {
		t.Fatalf("shopA unexpected err: %v", err)
	}
	if req.Service != "svcShopA" {
		t.Errorf("shopA service = %q, want svcShopA", req.Service)
	}

	req = testRequest("GET", "api.shopB.com", "/orders")
	if _, err := r.HandleHTTPFilt(req); err != nil {
		t.Fatalf("other host unexpected err: %v", err)
	}
	if req.Service != "svcDefault" {
		t.Errorf("other host service = %q, want svcDefault", req.Service)
	}
}

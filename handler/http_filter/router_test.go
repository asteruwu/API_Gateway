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
	r, err := NewRouter(builder.RouterConfig{
		Rules: []builder.RouterRule{
			{Hosts: []string{"api.shopA.com"}, PathPrefix: "/orders/refund", Service: "svcRefund"},
			{Hosts: []string{"api.shopA.com"}, PathPrefix: "/orders", Service: "svcOrder"},
			{Hosts: []string{"api.shopB.com"}, PathPrefix: "/orders", Service: "svcShopB"},
			{PathPrefix: "/", Service: "svcWeb"},
		},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	return r
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
	r, err := NewRouter(builder.RouterConfig{
		Rules: []builder.RouterRule{
			{Hosts: []string{"api.shopA.com"}, PathPrefix: "/orders", Service: "svcOrder"},
		},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

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
	r, err := NewRouter(builder.RouterConfig{
		Rules: []builder.RouterRule{
			{PathPrefix: "/orders", Methods: []string{"POST"}, Service: "svcPost"},
			{PathPrefix: "/", Service: "svcWeb"},
		},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

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
	r, err := NewRouter(builder.RouterConfig{
		Rules: []builder.RouterRule{
			{PathPrefix: "/orders", Service: "svcDefault"},
			{Hosts: []string{"api.shopA.com"}, PathPrefix: "/orders", Service: "svcShopA"},
		},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

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

// TestRouterPathNormalization 路径归一化：.. / . / 重复斜杠先归一化再匹配，
// 越权路径不落入更浅前缀，且 req.URL.Path 被改写为规范形态
func TestRouterPathNormalization(t *testing.T) {
	r := testRouter(t)

	cases := []struct {
		name string
		path string
		want string
	}{
		{"dotdot escapes prefix", "/orders/../admin", "svcWeb"},        // 归一化为 /admin，不命中 /orders
		{"dotdot inside prefix", "/orders/abc/../refund", "svcRefund"}, // 归一化为 /orders/refund
		{"duplicate slash", "/orders//abc", "svcOrder"},
		{"dot segment", "/orders/./abc", "svcOrder"},
	}

	for _, c := range cases {
		req := testRequest("GET", "api.shopA.com", c.path)
		if _, err := r.HandleHTTPFilt(req); err != nil {
			t.Errorf("%s: unexpected err: %v", c.name, err)
			continue
		}
		if req.Service != c.want {
			t.Errorf("%s: service = %q, want %q", c.name, req.Service, c.want)
		}
		if req.URL.Path != cleanPath(c.path) {
			t.Errorf("%s: URL.Path = %q, want %q", c.name, req.URL.Path, cleanPath(c.path))
		}
	}
}

// TestRouterPrefixSegmentBoundary 前缀必须落在路径段边界：/ordersXYZ 不命中 /orders
func TestRouterPrefixSegmentBoundary(t *testing.T) {
	r := testRouter(t)

	cases := []struct {
		path string
		want string
	}{
		{"/orders", "svcOrder"},      // 精确相等
		{"/orders/1", "svcOrder"},    // 段边界
		{"/ordersXYZ", "svcWeb"},     // 非段边界，不命中
		{"/orders-refund", "svcWeb"}, // 非段边界，不命中
	}

	for _, c := range cases {
		req := testRequest("GET", "api.shopA.com", c.path)
		if _, err := r.HandleHTTPFilt(req); err != nil {
			t.Errorf("%s: unexpected err: %v", c.path, err)
			continue
		}
		if req.Service != c.want {
			t.Errorf("%s: service = %q, want %q", c.path, req.Service, c.want)
		}
	}
}

// TestRouterHostNormalization host 比较忽略端口与大小写
func TestRouterHostNormalization(t *testing.T) {
	r := testRouter(t)

	for _, host := range []string{
		"api.shopA.com",
		"api.shopA.com:6666",
		"API.ShopA.COM",
		"API.SHOPA.COM:443",
	} {
		req := testRequest("GET", host, "/orders/1")
		if _, err := r.HandleHTTPFilt(req); err != nil {
			t.Errorf("%s: unexpected err: %v", host, err)
			continue
		}
		if req.Service != "svcOrder" {
			t.Errorf("%s: service = %q, want %q", host, req.Service, "svcOrder")
		}
	}
}

// TestNewRouterInvalidRule 规则自身不合法时 NewRouter 应直接拒绝构造
func TestNewRouterInvalidRule(t *testing.T) {
	cases := []struct {
		name  string
		rules []builder.RouterRule
	}{
		{"empty service", []builder.RouterRule{{PathPrefix: "/orders", Service: ""}}},
		{"missing leading slash", []builder.RouterRule{{PathPrefix: "orders", Service: "svcOrder"}}},
	}
	for _, c := range cases {
		if _, err := NewRouter(builder.RouterConfig{Rules: c.rules}); !errors.Is(err, gerrors.ErrInvalidRouterRule) {
			t.Errorf("%s: err = %v, want %v", c.name, err, gerrors.ErrInvalidRouterRule)
		}
	}
}

// TestRouterWritesRouteID 命中后 Router 应同时写入 route ID 与服务名，供下游按 RouteID 查策略
func TestRouterWritesRouteID(t *testing.T) {
	r, err := NewRouter(builder.RouterConfig{
		Rules: []builder.RouterRule{
			{ID: "shopA.order#0", Hosts: []string{"api.shopA.com"}, PathPrefix: "/orders", Service: "shopA.order"},
		},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	req := testRequest("GET", "api.shopA.com", "/orders/1")
	if _, err := r.HandleHTTPFilt(req); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if req.RouteID != "shopA.order#0" {
		t.Errorf("RouteID = %q, want %q", req.RouteID, "shopA.order#0")
	}
	if req.Service != "shopA.order" {
		t.Errorf("Service = %q, want %q", req.Service, "shopA.order")
	}
}

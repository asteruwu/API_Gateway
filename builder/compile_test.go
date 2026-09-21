package builder

import (
	"errors"
	"testing"

	gerrors "API_Gateway/pkg/errors"
)

func TestCompileSuccess(t *testing.T) {
	gw := GatewayConfig{Port: "6666"}
	platforms := []PlatformConfig{
		{
			Name: "shopA",
			Host: []string{"api.shopA.com"},
			Service: []ServiceConfig{
				{
					Name:      "order",
					Instances: []InstanceConfig{{Addr: "127.0.0.1:8081"}},
					Routes:    []RouteRuleConfig{{PathPrefix: "/orders", Methods: []string{"POST"}}},
				},
			},
		},
		{
			Name: "shopB",
			Host: []string{"api.shopB.com"},
			Service: []ServiceConfig{
				{
					Name:      "order", // 与 shopA 下的 order 同名，不同 platform，不应冲突
					Instances: []InstanceConfig{{Addr: "127.0.0.1:8091"}},
					Routes:    []RouteRuleConfig{{PathPrefix: "/orders"}},
				},
			},
		},
	}

	cfg, err := Compile(gw, platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	wantServices := map[string]string{
		"shopA.order": "127.0.0.1:8081",
		"shopB.order": "127.0.0.1:8091",
	}
	if len(cfg.Backend.Service) != len(wantServices) {
		t.Fatalf("backend services = %+v, want %d entries", cfg.Backend.Service, len(wantServices))
	}
	for _, ref := range cfg.Backend.Service {
		addr, ok := wantServices[ref.Name]
		if !ok {
			t.Errorf("unexpected service name %q", ref.Name)
			continue
		}
		if len(ref.Instances) != 1 || ref.Instances[0].Addr != addr {
			t.Errorf("service %q instances = %+v, want addr %q", ref.Name, ref.Instances, addr)
		}
	}

	// 路由规则应作为最后一个 http filter 追加，且 service 名已限定
	filters := cfg.Handler.Filter.Filters
	if len(filters) == 0 {
		t.Fatalf("expected router config appended to http filters")
	}
	routerCfg, ok := filters[len(filters)-1].(RouterConfig)
	if !ok {
		t.Fatalf("last http filter = %T, want RouterConfig", filters[len(filters)-1])
	}
	if len(routerCfg.Rules) != 2 {
		t.Fatalf("router rules = %+v, want 2 rules", routerCfg.Rules)
	}
	for _, rule := range routerCfg.Rules {
		switch {
		case len(rule.Hosts) == 1 && rule.Hosts[0] == "api.shopA.com":
			if rule.Service != "shopA.order" {
				t.Errorf("shopA rule service = %q, want shopA.order", rule.Service)
			}
		case len(rule.Hosts) == 1 && rule.Hosts[0] == "api.shopB.com":
			if rule.Service != "shopB.order" {
				t.Errorf("shopB rule service = %q, want shopB.order", rule.Service)
			}
		default:
			t.Errorf("unexpected rule hosts %+v", rule.Hosts)
		}
	}
}

func TestCompileDuplicatePlatformName(t *testing.T) {
	platforms := []PlatformConfig{
		{Name: "shopA"},
		{Name: "shopA"},
	}
	_, err := Compile(GatewayConfig{}, platforms)
	if !errors.Is(err, gerrors.ErrInvalidConfig) || !errors.Is(err, gerrors.ErrDuplicatePlatformName) {
		t.Errorf("err = %v, want wrapping %v and %v", err, gerrors.ErrInvalidConfig, gerrors.ErrDuplicatePlatformName)
	}
}

func TestCompileDuplicateServiceNameWithinPlatform(t *testing.T) {
	platforms := []PlatformConfig{
		{
			Name: "shopA",
			Service: []ServiceConfig{
				{Name: "order"},
				{Name: "order"},
			},
		},
	}
	_, err := Compile(GatewayConfig{}, platforms)
	if !errors.Is(err, gerrors.ErrDuplicateServiceName) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrDuplicateServiceName)
	}
}

func TestCompileServiceSameNameAcrossPlatformsAllowed(t *testing.T) {
	// 不同 platform 下同名 service 合法（限定名不同），单独验证不应触发 ErrDuplicateServiceName
	platforms := []PlatformConfig{
		{Name: "shopA", Service: []ServiceConfig{{Name: "order", Instances: []InstanceConfig{{Addr: "127.0.0.1:8081"}}}}},
		{Name: "shopB", Service: []ServiceConfig{{Name: "order", Instances: []InstanceConfig{{Addr: "127.0.0.1:8091"}}}}},
	}
	if _, err := Compile(GatewayConfig{}, platforms); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
}

func TestCompileEmptyServiceInstances(t *testing.T) {
	platforms := []PlatformConfig{
		{Name: "shopA", Service: []ServiceConfig{{Name: "order"}}}, // 未声明 Instances
	}
	_, err := Compile(GatewayConfig{}, platforms)
	if !errors.Is(err, gerrors.ErrEmptyServiceInstances) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrEmptyServiceInstances)
	}
}

func TestCompileDuplicateRoute(t *testing.T) {
	platforms := []PlatformConfig{
		{
			Name: "shopA",
			Host: []string{"api.shopA.com"},
			Service: []ServiceConfig{
				{Name: "order", Routes: []RouteRuleConfig{{PathPrefix: "/orders"}}},
				{Name: "order2", Routes: []RouteRuleConfig{{PathPrefix: "/orders"}}}, // 同 host+path，方法都为空（通配），冲突
			},
		},
	}
	_, err := Compile(GatewayConfig{}, platforms)
	if !errors.Is(err, gerrors.ErrDuplicateRoute) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrDuplicateRoute)
	}
}

func TestCompileRouteMethodPartitionAllowed(t *testing.T) {
	// 同 host+path，但方法集合不重叠（GET vs POST 分流到不同服务），不应判定冲突
	platforms := []PlatformConfig{
		{
			Name: "shopA",
			Service: []ServiceConfig{
				{Name: "reader", Instances: []InstanceConfig{{Addr: "127.0.0.1:8081"}}, Routes: []RouteRuleConfig{{PathPrefix: "/orders", Methods: []string{"GET"}}}},
				{Name: "writer", Instances: []InstanceConfig{{Addr: "127.0.0.1:8082"}}, Routes: []RouteRuleConfig{{PathPrefix: "/orders", Methods: []string{"POST"}}}},
			},
		},
	}
	if _, err := Compile(GatewayConfig{}, platforms); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
}

func TestCompilePluginSetOnlyEnabledAssembled(t *testing.T) {
	// 写进 Configs 不代表启用：只有出现在 Enabled 里的名字才会被装配进运行态配置
	gw := GatewayConfig{
		TCPFilters: PluginSet{
			Enabled: []string{"limit_a"},
			Configs: map[string]any{
				"limit_a": TCPLimiterFilterConfig{MaxConn: 10},
				"limit_b": TCPLimiterFilterConfig{MaxConn: 20}, // 未列入 Enabled，不应出现
			},
		},
	}
	cfg, err := Compile(gw, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(cfg.Connector.Filters) != 1 {
		t.Fatalf("connector filters = %+v, want 1 enabled entry", cfg.Connector.Filters)
	}
	got, ok := cfg.Connector.Filters[0].(TCPLimiterFilterConfig)
	if !ok || got.MaxConn != 10 {
		t.Errorf("connector filters[0] = %+v, want the enabled MaxConn=10 entry", cfg.Connector.Filters[0])
	}
}

func TestCompileHTTPPluginSetAssembledWithRouter(t *testing.T) {
	// http_filters 同理，且 compile 自动派生的 Router 始终追加在最后
	gw := GatewayConfig{
		HTTPFilters: PluginSet{
			Enabled: []string{"testF"},
			Configs: map[string]any{
				"testF":   struct{ Name string }{Name: "testF"},
				"unusedF": struct{ Name string }{Name: "unusedF"}, // 未列入 Enabled
			},
		},
	}
	cfg, err := Compile(gw, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	filters := cfg.Handler.Filter.Filters
	if len(filters) != 2 {
		t.Fatalf("http filters = %+v, want [testF, router]", filters)
	}
	if _, ok := filters[len(filters)-1].(RouterConfig); !ok {
		t.Errorf("last http filter = %T, want RouterConfig", filters[len(filters)-1])
	}
}

func TestCompilePluginSetEnabledMissingConfig(t *testing.T) {
	// Enabled 声明了某个名字，但 Configs 里找不到，属于编译期就能确定的疏忽
	gw := GatewayConfig{
		TCPFilters: PluginSet{
			Enabled: []string{"ghost"},
			Configs: map[string]any{},
		},
	}
	_, err := Compile(gw, nil)
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}

func TestCompileTransformerSetAssembledAsMap(t *testing.T) {
	// transformer 是按名字查找的注册表（不是顺序敏感的链），装配结果应保留 name → config 映射，
	// 未列入 Enabled 的 Configs 项不应出现
	gw := GatewayConfig{
		Transformer: PluginSet{
			Enabled: []string{"testT"},
			Configs: map[string]any{
				"testT":   TestTransformerConfig{},
				"unusedT": TestTransformerConfig{}, // 未列入 Enabled
			},
		},
	}
	cfg, err := Compile(gw, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	transformers := cfg.Handler.Transformer.Transformers
	if len(transformers) != 1 {
		t.Fatalf("transformers = %+v, want 1 enabled entry", transformers)
	}
	if _, ok := transformers["testT"]; !ok {
		t.Errorf("transformers = %+v, want key %q present", transformers, "testT")
	}
}

func TestCompileTransformerEnabledMissingConfig(t *testing.T) {
	gw := GatewayConfig{
		Transformer: PluginSet{
			Enabled: []string{"ghost"},
			Configs: map[string]any{},
		},
	}
	_, err := Compile(gw, nil)
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}

func TestCompileAggregatesAllErrors(t *testing.T) {
	// 多个问题同时存在时应一次性全部报出，而不是遇到第一个就退出
	platforms := []PlatformConfig{
		{Name: "shopA"},
		{Name: "shopA"}, // 重复 platform 名
		{Name: "shopB", Service: []ServiceConfig{{Name: "order"}, {Name: "order"}}}, // 重复 service 名
	}
	_, err := Compile(GatewayConfig{}, platforms)
	if !errors.Is(err, gerrors.ErrDuplicatePlatformName) {
		t.Errorf("err = %v, want also wrapping %v", err, gerrors.ErrDuplicatePlatformName)
	}
	if !errors.Is(err, gerrors.ErrDuplicateServiceName) {
		t.Errorf("err = %v, want also wrapping %v", err, gerrors.ErrDuplicateServiceName)
	}
}

package builder

import (
	"errors"
	"strings"
	"testing"

	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

func TestCompileSuccess(t *testing.T) {
	gw := GatewayConfig{
		Port:        "6666",
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins:     PluginsConfig{Transformer: map[string]any{"testT": TestTransformerConfig{}}},
	}
	platforms := []PlatformConfig{
		{
			Name: "shopA",
			Host: []string{"api.shopA.com"},
			Service: []ServiceConfig{
				{
					Name:        "order",
					Transformer: "testT",
					Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
					Routes:      []RouteRuleConfig{{PathPrefix: "/orders", Methods: []string{"POST"}}},
				},
			},
		},
		{
			Name: "shopB",
			Host: []string{"api.shopB.com"},
			Service: []ServiceConfig{
				{
					Name:        "order", // 与 shopA 下的 order 同名，不同 platform，不应冲突
					Transformer: "testT",
					Instances:   []InstanceConfig{{Addr: "127.0.0.1:8091"}},
					Routes:      []RouteRuleConfig{{PathPrefix: "/orders"}},
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

	// 路由规则应作为第一个 http filter 插入，且 service 名已限定
	filters := cfg.Handler.Filter.Filters
	if len(filters) == 0 {
		t.Fatalf("expected router config prepended to http filters")
	}
	routerCfg, ok := filters[0].(RouterConfig)
	if !ok {
		t.Fatalf("first http filter = %T, want RouterConfig", filters[0])
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
	gw := GatewayConfig{
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins:     PluginsConfig{Transformer: map[string]any{"testT": TestTransformerConfig{}}},
	}
	platforms := []PlatformConfig{
		{Name: "shopA", Service: []ServiceConfig{{Name: "order", Transformer: "testT", Instances: []InstanceConfig{{Addr: "127.0.0.1:8081"}}}}},
		{Name: "shopB", Service: []ServiceConfig{{Name: "order", Transformer: "testT", Instances: []InstanceConfig{{Addr: "127.0.0.1:8091"}}}}},
	}
	if _, err := Compile(gw, platforms); err != nil {
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
	gw := GatewayConfig{
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins:     PluginsConfig{Transformer: map[string]any{"testT": TestTransformerConfig{}}},
	}
	platforms := []PlatformConfig{
		{
			Name: "shopA",
			Service: []ServiceConfig{
				{Name: "reader", Transformer: "testT", Instances: []InstanceConfig{{Addr: "127.0.0.1:8081"}}, Routes: []RouteRuleConfig{{PathPrefix: "/orders", Methods: []string{"GET"}}}},
				{Name: "writer", Transformer: "testT", Instances: []InstanceConfig{{Addr: "127.0.0.1:8082"}}, Routes: []RouteRuleConfig{{PathPrefix: "/orders", Methods: []string{"POST"}}}},
			},
		},
	}
	if _, err := Compile(gw, platforms); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
}

func TestCompileOnlyEnabledSelected(t *testing.T) {
	// 写进注册表不代表启用：只有出现在 Enabled 里的名字才会被装配进运行态配置
	gw := GatewayConfig{
		TCPFilters: PluginActivation{Enabled: []string{"limit_a"}},
		Plugins: PluginsConfig{TCP: map[string]any{
			"limit_a": TCPLimiterFilterConfig{MaxConn: 10},
			"limit_b": TCPLimiterFilterConfig{MaxConn: 20}, // 未列入 Enabled，不应出现
		}},
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

func TestCompileHTTPSelectedWithRouter(t *testing.T) {
	// http_filters 同理，且 compile 自动派生的 Router 始终排在最前
	gw := GatewayConfig{
		HTTPFilters: PluginActivation{Enabled: []string{"testF"}},
		Plugins: PluginsConfig{HTTP: map[string]any{
			"testF":   struct{ Name string }{Name: "testF"},
			"unusedF": struct{ Name string }{Name: "unusedF"}, // 未列入 Enabled
		}},
	}
	cfg, err := Compile(gw, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	filters := cfg.Handler.Filter.Filters
	if len(filters) != 2 {
		t.Fatalf("http filters = %+v, want [router, testF]", filters)
	}
	if _, ok := filters[0].(RouterConfig); !ok {
		t.Errorf("first http filter = %T, want RouterConfig", filters[0])
	}
	configs := cfg.Handler.Filter.Configs
	if len(configs) != 1 {
		t.Fatalf("configs = %v, want only the enabled entry", configs)
	}
	if _, ok := configs["unusedF"]; ok {
		t.Errorf("configs unexpectedly contains %q", "unusedF")
	}
}

func TestCompileEnabledMissingConfig(t *testing.T) {
	// Enabled 声明了某个名字，但注册表里找不到，属于编译期就能确定的疏忽
	gw := GatewayConfig{
		TCPFilters: PluginActivation{Enabled: []string{"ghost"}},
	}
	_, err := Compile(gw, nil)
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}

func TestCompileHTTPEnabledMissingConfig(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginActivation{Enabled: []string{"ghost"}},
	}
	_, err := Compile(gw, nil)
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}

func TestCompileTransformerEnabledAssembledAsMap(t *testing.T) {
	// transformer 是按名字查找的注册表（不是顺序敏感的链），装配结果应保留 name → config 映射，
	// 未列入 Enabled 的注册表项不应出现
	gw := GatewayConfig{
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins: PluginsConfig{Transformer: map[string]any{
			"testT":   TestTransformerConfig{},
			"unusedT": TestTransformerConfig{}, // 未列入 Enabled
		}},
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
		Transformer: PluginActivation{Enabled: []string{"ghost"}},
	}
	_, err := Compile(gw, nil)
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}

func TestCompileTransformerBindsServices(t *testing.T) {
	// service 声明的 transformer 名应被反向聚合成该 transformer 的 Services 列表
	gw := GatewayConfig{
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins:     PluginsConfig{Transformer: map[string]any{"testT": TestTransformerConfig{}}},
	}
	platforms := []PlatformConfig{
		{Name: "shopA", Service: []ServiceConfig{
			{Name: "order", Transformer: "testT", Instances: []InstanceConfig{{Addr: "127.0.0.1:8081"}}},
			{Name: "user", Transformer: "testT", Instances: []InstanceConfig{{Addr: "127.0.0.1:8082"}}},
		}},
	}
	cfg, err := Compile(gw, platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	entry, ok := cfg.Handler.Transformer.Transformers["testT"]
	if !ok {
		t.Fatalf("transformers = %+v, want key %q", cfg.Handler.Transformer.Transformers, "testT")
	}
	if len(entry.Services) != 2 || entry.Services[0] != "shopA.order" || entry.Services[1] != "shopA.user" {
		t.Errorf("services = %v, want [shopA.order shopA.user]", entry.Services)
	}
}

func TestCompileServiceMissingTransformer(t *testing.T) {
	// service 未声明 transformer，属编译期可确定的配置错误
	platforms := []PlatformConfig{
		{Name: "shopA", Service: []ServiceConfig{
			{Name: "order", Instances: []InstanceConfig{{Addr: "127.0.0.1:8081"}}},
		}},
	}
	_, err := Compile(GatewayConfig{}, platforms)
	if !errors.Is(err, gerrors.ErrInvalidConfig) || !errors.Is(err, gerrors.ErrTransformerNotFound) {
		t.Errorf("err = %v, want wrapping %v and %v", err, gerrors.ErrInvalidConfig, gerrors.ErrTransformerNotFound)
	}
}

func TestCompileTransformerUnknownServiceRef(t *testing.T) {
	// service 绑定了未启用的 transformer 名，属编译期可确定的配置错误
	gw := GatewayConfig{
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins:     PluginsConfig{Transformer: map[string]any{"testT": TestTransformerConfig{}}},
	}
	platforms := []PlatformConfig{
		{Name: "shopA", Service: []ServiceConfig{
			{Name: "order", Transformer: "ghost", Instances: []InstanceConfig{{Addr: "127.0.0.1:8081"}}},
		}},
	}
	_, err := Compile(gw, platforms)
	if !errors.Is(err, gerrors.ErrInvalidConfig) || !errors.Is(err, gerrors.ErrTransformerNotFound) {
		t.Errorf("err = %v, want wrapping %v and %v", err, gerrors.ErrInvalidConfig, gerrors.ErrTransformerNotFound)
	}
}

// authGateway 返回一份启用了 auth 插件（含合法 JWT 参数）与 testT transformer 的网关配置
func authGateway() GatewayConfig {
	return GatewayConfig{
		HTTPFilters: PluginActivation{Enabled: []string{"auth"}},
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins: PluginsConfig{
			HTTP: map[string]any{"auth": AuthFilterConfig{
				Credentials: []string{gconst.AuthCredentialBearer},
				JWT: JWTVerifierConfig{
					SecretEnv: "JWT_SECRET",
				},
			}},
			Transformer: map[string]any{"testT": TestTransformerConfig{}},
		},
	}
}

// authPlatforms 返回一个含两条声明了 auth 的路由的平台配置
func authPlatforms() []PlatformConfig {
	return []PlatformConfig{{
		Name: "shopA",
		Host: []string{"api.shopA.com"},
		Service: []ServiceConfig{{
			Name:        "order",
			Transformer: "testT",
			Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
			Routes: []RouteRuleConfig{
				{PathPrefix: "/orders", Auth: &RouteAuthConfig{Credential: gconst.AuthCredentialBearer, Require: RouteAuthRequire{Roles: []string{"admin"}}}},
				{PathPrefix: "/orders/refund", Auth: &RouteAuthConfig{Credential: gconst.AuthCredentialBearer, Require: RouteAuthRequire{Roles: []string{"ops"}}}},
			},
		}},
	}}
}

// authConfigOf 从编译产物里取出 auth filter 的运行态配置
func authConfigOf(t *testing.T, cfg *Config) AuthFilterConfig {
	t.Helper()
	for _, f := range cfg.Handler.Filter.Filters {
		if ac, ok := f.(AuthFilterConfig); ok {
			return ac
		}
	}
	t.Fatalf("auth filter config not found in %+v", cfg.Handler.Filter.Filters)
	return AuthFilterConfig{}
}

// TestCompileRouteIDsAndAuthPolicies 校验 compile 为每条 route 生成唯一 ID，
// 并把 route 声明的 auth 反向聚合进 auth filter 的策略表（以 RouteID 为 key）
func TestCompileRouteIDsAndAuthPolicies(t *testing.T) {
	cfg, err := Compile(authGateway(), authPlatforms())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	routerCfg, ok := cfg.Handler.Filter.Filters[0].(RouterConfig)
	if !ok {
		t.Fatalf("first filter = %T, want RouterConfig", cfg.Handler.Filter.Filters[0])
	}
	if len(routerCfg.Rules) != 2 {
		t.Fatalf("router rules = %+v, want 2", routerCfg.Rules)
	}

	seen := make(map[string]bool, len(routerCfg.Rules))
	for _, r := range routerCfg.Rules {
		if r.ID == "" {
			t.Errorf("route ID is empty for path %q", r.PathPrefix)
		}
		if seen[r.ID] {
			t.Errorf("duplicate route ID %q", r.ID)
		}
		seen[r.ID] = true
	}

	ac := authConfigOf(t, cfg)
	if len(ac.Policies) != 2 {
		t.Fatalf("policies = %+v, want 2 entries", ac.Policies)
	}
	wantRoles := map[string]string{
		"shopA.order#0": "admin",
		"shopA.order#1": "ops",
	}
	for id, role := range wantRoles {
		p, ok := ac.Policies[id]
		if !ok {
			t.Errorf("policy missing for route %q", id)
			continue
		}
		if len(p.Roles) != 1 || p.Roles[0] != role {
			t.Errorf("policy %q roles = %v, want [%s]", id, p.Roles, role)
		}
	}
}

// TestCompileRouteAuthWithoutPlugin 路由声明了 auth 但未启用 auth 插件，编译期应拒绝
func TestCompileRouteAuthWithoutPlugin(t *testing.T) {
	gw := authGateway()
	gw.HTTPFilters = PluginActivation{} // 默认链为空，等于不启用 auth

	_, err := Compile(gw, authPlatforms())
	if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
	}
}

// TestCompileAuthEnabledWithoutRouteAuth 启用了 auth 却没有任何路由声明 auth，
// 运行期会全量拒绝，属编译期可确定的错误
func TestCompileAuthEnabledWithoutRouteAuth(t *testing.T) {
	platforms := []PlatformConfig{{
		Name: "shopA",
		Service: []ServiceConfig{{
			Name:        "order",
			Transformer: "testT",
			Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
			Routes:      []RouteRuleConfig{{PathPrefix: "/orders"}},
		}},
	}}

	_, err := Compile(authGateway(), platforms)
	if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
	}
}

// TestCompileAuthUnsupportedCredential 不支持的凭证类型应被拒绝
func TestCompileAuthUnsupportedCredential(t *testing.T) {
	gw := authGateway()
	ac := gw.Plugins.HTTP["auth"].(AuthFilterConfig)
	ac.Credentials = []string{"basic"}
	gw.Plugins.HTTP["auth"] = ac

	_, err := Compile(gw, authPlatforms())
	if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
	}
}

// TestCompileAuthInvalidJWT 缺少 secret_env 应被拒绝
func TestCompileAuthInvalidJWT(t *testing.T) {
	gw := authGateway()
	ac := gw.Plugins.HTTP["auth"].(AuthFilterConfig)
	ac.JWT = JWTVerifierConfig{} // 缺 secret_env
	gw.Plugins.HTTP["auth"] = ac

	_, err := Compile(gw, authPlatforms())
	if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
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

// oneRoutePlatform 构造一个只含一条 route 的平台，便于单条声明场景的用例
func oneRoutePlatform(route RouteRuleConfig) []PlatformConfig {
	return []PlatformConfig{{
		Name: "shopA",
		Host: []string{"api.shopA.com"},
		Service: []ServiceConfig{{
			Name:        "order",
			Transformer: "testT",
			Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
			Routes:      []RouteRuleConfig{route},
		}},
	}}
}

// TestCompileRouteAuthMissingDeclaration 启用 auth 时，未声明 auth 的 route 必须被拒绝（fail-closed）
func TestCompileRouteAuthMissingDeclaration(t *testing.T) {
	platforms := []PlatformConfig{{
		Name: "shopA",
		Host: []string{"api.shopA.com"},
		Service: []ServiceConfig{{
			Name:        "order",
			Transformer: "testT",
			Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
			Routes: []RouteRuleConfig{
				{PathPrefix: "/orders", Auth: &RouteAuthConfig{Credential: gconst.AuthCredentialBearer}},
				{PathPrefix: "/orders/refund"}, // 漏声明
			},
		}},
	}}

	_, err := Compile(authGateway(), platforms)
	if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
	}
}

// TestCompileRouteAuthPublicExclusive public 与认证/授权要求互斥
func TestCompileRouteAuthPublicExclusive(t *testing.T) {
	cases := map[string]RouteAuthConfig{
		"public with credential": {Public: true, Credential: gconst.AuthCredentialBearer},
		"public with roles":      {Public: true, Require: RouteAuthRequire{Roles: []string{"admin"}}},
		"public with scopes":     {Public: true, Require: RouteAuthRequire{Scopes: []string{"read"}}},
	}
	for name, auth := range cases {
		t.Run(name, func(t *testing.T) {
			auth := auth
			_, err := Compile(authGateway(), oneRoutePlatform(RouteRuleConfig{PathPrefix: "/orders", Auth: &auth}))
			if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
				t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
			}
		})
	}
}

// TestCompileRouteAuthMissingCredential 非匿名 route 未声明 credential 应被拒绝
func TestCompileRouteAuthMissingCredential(t *testing.T) {
	_, err := Compile(authGateway(), oneRoutePlatform(RouteRuleConfig{
		PathPrefix: "/orders",
		Auth:       &RouteAuthConfig{Require: RouteAuthRequire{Roles: []string{"admin"}}},
	}))
	if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
	}
}

// TestCompileRouteAuthCredentialNotAccepted route 要求的 credential 不在网关接受列表内应被拒绝
func TestCompileRouteAuthCredentialNotAccepted(t *testing.T) {
	_, err := Compile(authGateway(), oneRoutePlatform(RouteRuleConfig{
		PathPrefix: "/orders",
		Auth:       &RouteAuthConfig{Credential: "basic"},
	}))
	if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
	}
}

// TestCompileAuthPoliciesFullTable 策略表对每条 route 都产出条目（含 public），
// 且编写态分层声明被展平为运行态的并列字段
func TestCompileAuthPoliciesFullTable(t *testing.T) {
	platforms := []PlatformConfig{{
		Name: "shopA",
		Host: []string{"api.shopA.com"},
		Service: []ServiceConfig{{
			Name:        "order",
			Transformer: "testT",
			Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
			Routes: []RouteRuleConfig{
				{PathPrefix: "/public", Auth: &RouteAuthConfig{Public: true}},
				{PathPrefix: "/orders", Auth: &RouteAuthConfig{Credential: gconst.AuthCredentialBearer, Require: RouteAuthRequire{Roles: []string{"admin"}}}},
				{PathPrefix: "/reports", Auth: &RouteAuthConfig{Credential: gconst.AuthCredentialBearer, Require: RouteAuthRequire{Scopes: []string{"read"}}}},
			},
		}},
	}}

	cfg, err := Compile(authGateway(), platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	ac := authConfigOf(t, cfg)
	if len(ac.Policies) != 3 {
		t.Fatalf("policies = %+v, want 3 entries (one per route)", ac.Policies)
	}

	pub := ac.Policies["shopA.order#0"]
	if !pub.Public || pub.Credential != "" || len(pub.Roles) != 0 || len(pub.Scopes) != 0 {
		t.Errorf("public policy = %+v, want only Public=true", pub)
	}
	roles := ac.Policies["shopA.order#1"]
	if roles.Public || roles.Credential != gconst.AuthCredentialBearer || len(roles.Roles) != 1 || roles.Roles[0] != "admin" {
		t.Errorf("roles policy = %+v, want credential bearer + roles [admin]", roles)
	}
	scopes := ac.Policies["shopA.order#2"]
	if len(scopes.Scopes) != 1 || scopes.Scopes[0] != "read" {
		t.Errorf("scopes policy = %+v, want scopes [read]", scopes)
	}
}

// TestCompilePipelineInheritsDefault route 不写 filters，pipeline 继承 enabled 顺序
func TestCompilePipelineInheritsDefault(t *testing.T) {
	gw := authGateway()
	platforms := authPlatforms()

	cfg, err := Compile(gw, platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for id, pl := range cfg.Handler.Filter.Pipelines {
		if len(pl) != 1 || pl[0] != "auth" {
			t.Errorf("pipeline[%s] = %v, want [auth]", id, pl)
		}
	}
}

// TestCompilePipelineExplicitOverride route 声明 filters 后以声明为准
func TestCompilePipelineExplicitOverride(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginActivation{Enabled: []string{"testF"}},
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins: PluginsConfig{
			HTTP: map[string]any{
				"testF":  struct{}{},
				"testF2": struct{}{},
			},
			Transformer: map[string]any{"testT": TestTransformerConfig{}},
		},
	}
	platforms := []PlatformConfig{{
		Name: "p",
		Service: []ServiceConfig{{
			Name:        "s",
			Transformer: "testT",
			Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
			Routes: []RouteRuleConfig{
				{PathPrefix: "/default"}, // nil → 继承
				{PathPrefix: "/custom", Filters: []string{"testF2", "testF"}}, // 显式覆盖
				{PathPrefix: "/empty", Filters: []string{}},                   // 显式空链
			},
		}},
	}}

	cfg, err := Compile(gw, platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	pl := cfg.Handler.Filter.Pipelines

	if got := pl["p.s#0"]; len(got) != 1 || got[0] != "testF" {
		t.Errorf("default pipeline = %v, want [testF]", got)
	}
	if got := pl["p.s#1"]; len(got) != 2 || got[0] != "testF2" || got[1] != "testF" {
		t.Errorf("custom pipeline = %v, want [testF2 testF]", got)
	}
	if got := pl["p.s#2"]; len(got) != 0 {
		t.Errorf("empty pipeline = %v, want []", got)
	}
}

// TestCompileConfigsContainsUnion 运行态 Configs 包含默认链 ∪ route 声明的 filter
func TestCompileConfigsContainsUnion(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginActivation{Enabled: []string{"testF"}},
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins: PluginsConfig{
			HTTP: map[string]any{
				"testF":  struct{ N string }{N: "enabled"},
				"testF2": struct{ N string }{N: "route-only"},
			},
			Transformer: map[string]any{"testT": TestTransformerConfig{}},
		},
	}
	platforms := []PlatformConfig{{
		Name: "p",
		Service: []ServiceConfig{{
			Name:        "s",
			Transformer: "testT",
			Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
			Routes:      []RouteRuleConfig{{PathPrefix: "/a", Filters: []string{"testF2"}}},
		}},
	}}

	cfg, err := Compile(gw, platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	configs := cfg.Handler.Filter.Configs
	if len(configs) != 2 {
		t.Fatalf("configs = %v, want 2 entries (union)", configs)
	}
	if _, ok := configs["testF"]; !ok {
		t.Errorf("configs missing enabled filter %q", "testF")
	}
	if _, ok := configs["testF2"]; !ok {
		t.Errorf("configs missing route-only filter %q", "testF2")
	}
}

// TestCompileRouterFieldPopulated HTTPFilterConfig.Router 应和 Filters[0] 里的一致
func TestCompileRouterFieldPopulated(t *testing.T) {
	gw := GatewayConfig{
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins:     PluginsConfig{Transformer: map[string]any{"testT": TestTransformerConfig{}}},
	}
	platforms := []PlatformConfig{{
		Name: "p",
		Service: []ServiceConfig{{
			Name:        "s",
			Transformer: "testT",
			Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
			Routes:      []RouteRuleConfig{{PathPrefix: "/a"}},
		}},
	}}

	cfg, err := Compile(gw, platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(cfg.Handler.Filter.Router.Rules) != 1 {
		t.Errorf("Router.Rules = %+v, want 1 rule", cfg.Handler.Filter.Router.Rules)
	}
}

// TestCompileRoutePipelineAuthMismatch pipeline 与 auth 声明不一致，两个方向都要拒绝
func TestCompileRoutePipelineAuthMismatch(t *testing.T) {
	cases := []struct {
		name  string
		gw    GatewayConfig
		route RouteRuleConfig
	}{
		{
			name:  "pipeline has auth but no auth block",
			gw:    authGateway(),
			route: RouteRuleConfig{PathPrefix: "/orders"},
		},
		{
			name:  "auth block but pipeline lacks auth",
			gw:    authGateway(),
			route: RouteRuleConfig{PathPrefix: "/public", Filters: []string{"other"}, Auth: &RouteAuthConfig{Public: true}},
		},
		{
			name:  "explicit empty chain with auth block",
			gw:    authGateway(),
			route: RouteRuleConfig{PathPrefix: "/public", Filters: []string{}, Auth: &RouteAuthConfig{Credential: gconst.AuthCredentialBearer}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gw := c.gw
			// 备用 filter 需要有配置，否则先被 hydrate 前置条件挡住，测不到 auth 一致性
			if gw.Plugins.HTTP == nil {
				gw.Plugins.HTTP = map[string]any{}
			}
			gw.Plugins.HTTP["other"] = struct{}{}

			_, err := Compile(gw, oneRoutePlatform(c.route))
			if !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
				t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
			}
		})
	}
}

// TestCompileRoutePipelineAuthBothAbsent pipeline 与 auth 声明都没有，route 即为无鉴权
func TestCompileRoutePipelineAuthBothAbsent(t *testing.T) {
	platforms := oneRoutePlatform(RouteRuleConfig{PathPrefix: "/health", Filters: []string{}})

	cfg, err := Compile(authGateway(), platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := cfg.Handler.Filter.Pipelines["shopA.order#0"]; len(got) != 0 {
		t.Errorf("pipeline = %v, want empty", got)
	}
}

// TestCompileRouteFiltersDuplicate 同一条 route 重复声明同一个 filter 应报错
func TestCompileRouteFiltersDuplicate(t *testing.T) {
	gw := authGateway()
	_, err := Compile(gw, oneRoutePlatform(RouteRuleConfig{
		PathPrefix: "/orders",
		Filters:    []string{"auth", "auth"},
		Auth:       &RouteAuthConfig{Credential: gconst.AuthCredentialBearer},
	}))
	if !errors.Is(err, gerrors.ErrInvalidConfig) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidConfig)
	}
	if err == nil || !strings.Contains(err.Error(), "repeats filter") {
		t.Errorf("err = %v, want mention repeats filter", err)
	}
}

// TestCompileAuthReferencedOnlyByRoute auth 不进 enabled、只被 route 引用也应成立
func TestCompileAuthReferencedOnlyByRoute(t *testing.T) {
	gw := GatewayConfig{
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins: PluginsConfig{
			HTTP: map[string]any{"auth": AuthFilterConfig{
				Credentials: []string{gconst.AuthCredentialBearer},
				JWT:         JWTVerifierConfig{SecretEnv: "JWT_SECRET"},
			}},
			Transformer: map[string]any{"testT": TestTransformerConfig{}},
		},
	}
	platforms := oneRoutePlatform(RouteRuleConfig{
		PathPrefix: "/orders",
		Filters:    []string{"auth"},
		Auth:       &RouteAuthConfig{Credential: gconst.AuthCredentialBearer},
	})

	cfg, err := Compile(gw, platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := cfg.Handler.Filter.Pipelines["shopA.order#0"]; len(got) != 1 || got[0] != "auth" {
		t.Errorf("pipeline = %v, want [auth]", got)
	}
	if _, ok := cfg.Handler.Filter.Configs["auth"].(AuthFilterConfig); !ok {
		t.Errorf("configs[auth] = %T, want AuthFilterConfig", cfg.Handler.Filter.Configs["auth"])
	}
	// enabled 为空，过渡期旧链里只剩 Router
	if len(cfg.Handler.Filter.Filters) != 1 {
		t.Errorf("legacy filters = %+v, want only router", cfg.Handler.Filter.Filters)
	}
}

// TestCompileRouteReferencedFilterMissingConfig route 引用了 Configs 里没有的 filter
func TestCompileRouteReferencedFilterMissingConfig(t *testing.T) {
	_, err := Compile(GatewayConfig{}, oneRoutePlatform(RouteRuleConfig{
		PathPrefix: "/orders",
		Filters:    []string{"ghost"},
	}))
	if !errors.Is(err, gerrors.ErrFilterConfigNotFound) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrFilterConfigNotFound)
	}
}

// TestCompilePipelineCopiedPerRoute 多条 route 继承同一条默认链时必须各自独立
func TestCompilePipelineCopiedPerRoute(t *testing.T) {
	gw := GatewayConfig{
		HTTPFilters: PluginActivation{Enabled: []string{"f1", "f2"}},
		Transformer: PluginActivation{Enabled: []string{"testT"}},
		Plugins: PluginsConfig{
			HTTP:        map[string]any{"f1": struct{}{}, "f2": struct{}{}},
			Transformer: map[string]any{"testT": TestTransformerConfig{}},
		},
	}
	platforms := []PlatformConfig{{
		Name: "shopA",
		Service: []ServiceConfig{{
			Name:        "order",
			Transformer: "testT",
			Instances:   []InstanceConfig{{Addr: "127.0.0.1:8081"}},
			Routes: []RouteRuleConfig{
				{PathPrefix: "/a"},
				{PathPrefix: "/b"},
			},
		}},
	}}

	cfg, err := Compile(gw, platforms)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	p0 := cfg.Handler.Filter.Pipelines["shopA.order#0"]
	p1 := cfg.Handler.Filter.Pipelines["shopA.order#1"]
	p0[0] = "mutated"
	if p1[0] != "f1" {
		t.Errorf("pipeline[#1] = %v, want untouched [f1 f2]", p1)
	}
}

// TestCompileAuthPoliciesInConfigs auth 策略必须同时写入 Configs 里的 AuthFilterConfig
func TestCompileAuthPoliciesInConfigs(t *testing.T) {
	cfg, err := Compile(authGateway(), authPlatforms())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	ac, ok := cfg.Handler.Filter.Configs["auth"].(AuthFilterConfig)
	if !ok {
		t.Fatalf("configs[auth] = %T, want AuthFilterConfig", cfg.Handler.Filter.Configs["auth"])
	}
	if len(ac.Policies) != 2 {
		t.Errorf("policies = %+v, want 2 entries", ac.Policies)
	}
}

package builder

import (
	"fmt"
	"slices"

	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

// ValidateRuntime 校验运行态语义
func ValidateRuntime(gw GatewayConfig, plan *Plan) error {
	if plan == nil {
		return configErrors("runtime", []error{fmt.Errorf("%w: plan is nil", gerrors.ErrInvalidConfig)})
	}

	errs := validateAuthPlugin(gw, plan)
	if len(errs) == 0 {
		return nil
	}
	return configErrors("runtime", errs)
}

// validateAuthPlugin 校验水合后的 auth 插件配置，以及 route credential 是否被接受
func validateAuthPlugin(gw GatewayConfig, plan *Plan) []error {
	if !slices.Contains(plan.HTTPFilters, "auth") {
		return nil
	}

	cfg, ok := gw.Plugins.HTTP["auth"].(AuthFilterConfig)
	if !ok {
		return []error{fmt.Errorf("%w: auth plugin config missing or of wrong type %T",
			gerrors.ErrInvalidAuthConfig, gw.Plugins.HTTP["auth"])}
	}

	errs := validateCredentials(cfg.Credentials)
	errs = append(errs, validateJWTVerifier(cfg.JWT)...)
	errs = append(errs, validateRouteCredentials(plan, cfg.Credentials)...)
	return errs
}

// validateRouteCredentials 校验走 auth pipeline 的非匿名 route，其 credential 被 auth filter 接受
func validateRouteCredentials(plan *Plan, accepted []string) []error {
	var errs []error
	for _, r := range plan.Routes {
		if !slices.Contains(r.Pipeline, "auth") || r.Policy.Public {
			continue
		}
		if !slices.Contains(accepted, r.Policy.Credential) {
			errs = append(errs, fmt.Errorf("%w: route %q requires credential %q not accepted by auth filter %v",
				gerrors.ErrInvalidAuthConfig, r.Rule.ID, r.Policy.Credential, accepted))
		}
	}
	return errs
}

// validateCredentials 校验凭证类型都受支持
func validateCredentials(creds []string) []error {
	if len(creds) == 0 {
		return []error{fmt.Errorf("%w: credentials must not be empty", gerrors.ErrInvalidAuthConfig)}
	}
	var errs []error
	for _, c := range creds {
		if c != gconst.AuthCredentialBearer {
			errs = append(errs, fmt.Errorf("%w: unsupported credential %q", gerrors.ErrInvalidAuthConfig, c))
		}
	}
	return errs
}

// validateJWTVerifier 校验 JWT 校验参数：算法受支持、密钥来源已声明、容差非负
func validateJWTVerifier(cfg JWTVerifierConfig) []error {
	var errs []error
	if cfg.SecretEnv == "" {
		errs = append(errs, fmt.Errorf("%w: jwt.secret_env is required", gerrors.ErrInvalidAuthConfig))
	}
	if cfg.Leeway < 0 {
		errs = append(errs, fmt.Errorf("%w: jwt.leeway must not be negative", gerrors.ErrInvalidAuthConfig))
	}
	return errs
}

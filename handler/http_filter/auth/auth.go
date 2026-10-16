package auth

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"API_Gateway/builder"
	"API_Gateway/handler/http_filter/auth/verifier"
	"API_Gateway/handler/http_filter/auth/verifier/jwt"
	"API_Gateway/handler/message"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
)

const prefix = "Bearer "

type AuthFilter struct {
	verifier verifier.Verifier
	policies map[string]builder.AuthPolicy
}

// NewAuthFilter 从编译好的 auth 插件配置构造 filter：初始化 verifier（解析声明式密钥）+ 接管策略表。
func NewAuthFilter(cfg builder.AuthFilterConfig) (*AuthFilter, error) {
	v, err := jwt.NewJWTVerifier(cfg.JWT)
	if err != nil {
		return nil, err
	}
	return &AuthFilter{verifier: v, policies: cfg.Policies}, nil
}

func (f *AuthFilter) HandleHTTPFilt(req *message.Request) (*message.Response, error) {
	stripReservedHeaders(req.Header)

	policy, ok := f.policies[req.RouteID]
	if !ok {
		// 命中路由却没有策略：fail-closed
		return nil, fmt.Errorf("%w: no auth policy for route %q", gerrors.ErrForbidden, req.RouteID)
	}
	if policy.Public {
		return nil, nil
	}

	// ① 凭证提取
	raw, err := extractCredential(req, policy.Credential)
	if err != nil {
		return nil, err
	}
	// ② 认证
	verified, err := f.verifier.Verify(raw)
	if err != nil {
		return nil, err
	}
	// ③ 授权：维度检查（声明为空的维度跳过）
	if err := checkRoles(policy.Roles, verified.Roles); err != nil {
		return nil, err
	}
	if err := checkScopes(policy.Scopes, verified.Scopes); err != nil {
		return nil, err
	}
	// ④ 回写身份
	req.Identity = &message.Identity{Subject: verified.Subject, Roles: verified.Roles}
	return nil, nil
}

// extractCredential 按策略声明的凭证类型从请求提取原始凭证
func extractCredential(req *message.Request, credential string) (string, error) {
	switch credential {
	case gconst.AuthCredentialBearer:
		token, ok := bearerToken(req.Header.Get("Authorization"))
		if !ok {
			return "", fmt.Errorf("%w: missing bearer credential", gerrors.ErrUnauthorized)
		}
		return token, nil
	default:
		return "", fmt.Errorf("%w: unsupported credential %q", gerrors.ErrUnauthorized, credential)
	}
}

// bearerToken 从 Authorization 头解析 Bearer 凭证
func bearerToken(header string) (string, bool) {
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}

// checkRoles 角色维度：要求为空则跳过，否则至少命中一个
func checkRoles(required, granted []string) error {
	if len(required) == 0 {
		return nil
	}
	for _, r := range required {
		if slices.Contains(granted, r) {
			return nil
		}
	}
	return fmt.Errorf("%w: missing required role %v", gerrors.ErrForbidden, required)
}

// checkScopes 权限维度：要求为空则跳过，否则至少命中一个
func checkScopes(required, granted []string) error {
	if len(required) == 0 {
		return nil
	}
	for _, s := range required {
		if slices.Contains(granted, s) {
			return nil
		}
	}
	return fmt.Errorf("%w: missing required scope %v", gerrors.ErrForbidden, required)
}

// stripReservedHeaders 剥离入站所有网关保留命名空间（前缀）下的头，防止客户端伪造身份透传到后端
func stripReservedHeaders(header http.Header) {
	for name := range header {
		if isReservedHeader(name) {
			delete(header, name)
		}
	}
}

func isReservedHeader(name string) bool {
	prefix := gconst.GatewayHeaderPrefix
	if len(name) < len(prefix) {
		return false
	}
	return strings.EqualFold(name[:len(prefix)], prefix)
}

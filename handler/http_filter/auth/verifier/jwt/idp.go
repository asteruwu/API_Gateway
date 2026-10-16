package jwt

import (
	"strings"

	verifier "API_Gateway/handler/http_filter/auth/verifier"
	"API_Gateway/pkg/tools"
)

// idpLayout 一种 IdP 的 claim 布局：如何识别、如何抽取身份
type idpLayout struct {
	name    string
	match   func(issuer string) bool
	extract func(claims map[string]any) *verifier.Verified
}

// idpLayouts 内置布局表；generic 必须放最后作为兜底
var idpLayouts = []idpLayout{
	{name: "keycloak", match: isKeycloak, extract: extractKeycloak},
	{name: "azure", match: isAzure, extract: extractAzure},
	{name: "generic", match: func(string) bool { return true }, extract: extractGeneric},
}

// resolveLayout 按 iss 识别 IdP，返回对应布局（未识别则回退 generic）
func resolveLayout(claims map[string]any) idpLayout {
	iss, _ := claims["iss"].(string)
	for _, l := range idpLayouts {
		if l.match(iss) {
			return l
		}
	}
	return idpLayouts[len(idpLayouts)-1]
}

// isKeycloak Keycloak 的 issuer 形如 https://host/realms/<realm>
func isKeycloak(issuer string) bool {
	return strings.Contains(issuer, "/realms/")
}

// isAzure Azure AD / Entra 的 issuer 取自 login.microsoftonline.com 或 sts.windows.net
func isAzure(issuer string) bool {
	return strings.Contains(issuer, "login.microsoftonline.com") || strings.Contains(issuer, "sts.windows.net")
}

// extractGeneric 通用 OIDC：顶层 roles + scope/scp
func extractGeneric(claims map[string]any) *verifier.Verified {
	return &verifier.Verified{
		Subject: subjectOf(claims),
		Roles:   tools.ToStringsLenient(claims["roles"]),
		Scopes:  scopesFrom(claims, "scope", "scp"),
	}
}

// extractKeycloak Keycloak：角色在嵌套的 realm_access.roles
func extractKeycloak(claims map[string]any) *verifier.Verified {
	realm, _ := claims["realm_access"].(map[string]any)
	return &verifier.Verified{
		Subject: subjectOf(claims),
		Roles:   tools.ToStringsLenient(realm["roles"]),
		Scopes:  scopesFrom(claims, "scope", "scp"),
	}
}

// extractAzure Azure AD：权限在 scp，角色在顶层 roles
func extractAzure(claims map[string]any) *verifier.Verified {
	return &verifier.Verified{
		Subject: subjectOf(claims),
		Roles:   tools.ToStringsLenient(claims["roles"]),
		Scopes:  scopesFrom(claims, "scp", "scope"),
	}
}

func subjectOf(claims map[string]any) string {
	sub, _ := claims["sub"].(string)
	return sub
}

// scopesFrom 依次尝试 primary/secondary 两个 claim，兼容空格分隔字符串与数组两种形态
func scopesFrom(claims map[string]any, primary, secondary string) []string {
	if s, ok := claims[primary].(string); ok && s != "" {
		return strings.Fields(s)
	}
	if scopes := tools.ToStringsLenient(claims[primary]); len(scopes) > 0 {
		return scopes
	}
	return tools.ToStringsLenient(claims[secondary])
}

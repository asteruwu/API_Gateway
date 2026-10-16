package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"API_Gateway/builder"
	gerrors "API_Gateway/pkg/errors"
)

const (
	testSecretEnv = "TEST_JWT_SECRET"
	testSecret    = "0123456789abcdef0123456789abcdef" // 32 字节
)

// newVerifier 固定密钥环境变量后按配置构造校验器
func newVerifier(t *testing.T, cfg builder.JWTVerifierConfig) *JWTVerifier {
	t.Helper()
	t.Setenv(testSecretEnv, testSecret)
	if cfg.SecretEnv == "" {
		cfg.SecretEnv = testSecretEnv
	}
	v, err := NewJWTVerifier(cfg)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return v
}

// signJWT 用给定 secret 对 header/claims 做 HS256 签名，产出一个合法 JWT
func signJWT(t *testing.T, secret []byte, header, claims map[string]any) string {
	t.Helper()
	hb, err := json.Marshal(header)
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

// baseClaims 返回一份含必需 exp 的基础 claim 集
func baseClaims(exp time.Time) map[string]any {
	return map[string]any{"exp": float64(exp.Unix())}
}

// TestNewJWTVerifierRejectsShortSecret 密钥缺失或短于下限时应拒绝构造
func TestNewJWTVerifierRejectsShortSecret(t *testing.T) {
	t.Setenv("SHORT_JWT_SECRET", "short")
	for name, env := range map[string]string{"missing": "EMPTY_JWT_SECRET", "short": "SHORT_JWT_SECRET"} {
		t.Run(name, func(t *testing.T) {
			if env == "EMPTY_JWT_SECRET" {
				t.Setenv(env, "")
			}
			if _, err := NewJWTVerifier(builder.JWTVerifierConfig{SecretEnv: env}); !errors.Is(err, gerrors.ErrInvalidAuthConfig) {
				t.Errorf("err = %v, want %v", err, gerrors.ErrInvalidAuthConfig)
			}
		})
	}
}

// TestJWTVerifyValid 合法 token 应归一化出 Subject/Roles/Scopes
func TestJWTVerifyValid(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{})
	token := signJWT(t, []byte(testSecret),
		map[string]any{"alg": "HS256", "typ": "JWT"},
		map[string]any{
			"sub":   "user-123",
			"roles": []any{"admin", "ops"},
			"scope": "orders:read orders:write",
			"exp":   float64(time.Now().Add(time.Hour).Unix()),
		})

	got, err := v.Verify(token)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", got.Subject)
	}
	if !slices.Equal(got.Roles, []string{"admin", "ops"}) {
		t.Errorf("Roles = %v, want [admin ops]", got.Roles)
	}
	if !slices.Equal(got.Scopes, []string{"orders:read", "orders:write"}) {
		t.Errorf("Scopes = %v, want [orders:read orders:write]", got.Scopes)
	}
}

// TestJWTVerifyExpired 过期 token（超出 leeway）应被拒绝
func TestJWTVerifyExpired(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{})
	token := signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, baseClaims(time.Now().Add(-time.Minute)))

	if _, err := v.Verify(token); !errors.Is(err, gerrors.ErrUnauthorized) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrUnauthorized)
	}
}

// TestJWTVerifyLeeway leeway 内的过期 token 应放行，超出则拒绝
func TestJWTVerifyLeeway(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{Leeway: 5 * time.Second})

	ok := signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, baseClaims(time.Now().Add(-time.Second)))
	if _, err := v.Verify(ok); err != nil {
		t.Errorf("within leeway: unexpected err: %v", err)
	}

	bad := signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, baseClaims(time.Now().Add(-time.Minute)))
	if _, err := v.Verify(bad); !errors.Is(err, gerrors.ErrUnauthorized) {
		t.Errorf("beyond leeway: err = %v, want %v", err, gerrors.ErrUnauthorized)
	}
}

// TestJWTVerifyMissingExp 缺少 exp 的 token 应被拒绝（fail-closed）
func TestJWTVerifyMissingExp(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{})
	token := signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, map[string]any{"sub": "u"})

	if _, err := v.Verify(token); !errors.Is(err, gerrors.ErrUnauthorized) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrUnauthorized)
	}
}

// TestJWTVerifyTampered 篡改签名后应被拒绝
func TestJWTVerifyTampered(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{})
	token := signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, baseClaims(time.Now().Add(time.Hour)))

	// 改签名段首字符：该字符编码真实数据位，解码后必然变化 → 验签失败
	parts := strings.Split(token, ".")
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(sig)

	if _, err := v.Verify(tampered); !errors.Is(err, gerrors.ErrUnauthorized) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrUnauthorized)
	}
}

// TestJWTVerifyWrongAlg 非 HS256 的算法声明应被拒绝
func TestJWTVerifyWrongAlg(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{})
	token := signJWT(t, []byte(testSecret), map[string]any{"alg": "none"}, baseClaims(time.Now().Add(time.Hour)))

	if _, err := v.Verify(token); !errors.Is(err, gerrors.ErrUnauthorized) {
		t.Errorf("err = %v, want %v", err, gerrors.ErrUnauthorized)
	}
}

// TestJWTVerifyIssuer 配置 issuer 后应校验 iss
func TestJWTVerifyIssuer(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{Issuer: "https://issuer"})

	claims := baseClaims(time.Now().Add(time.Hour))
	claims["iss"] = "https://issuer"
	if _, err := v.Verify(signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, claims)); err != nil {
		t.Errorf("matching issuer: unexpected err: %v", err)
	}

	claims["iss"] = "https://evil"
	if _, err := v.Verify(signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, claims)); !errors.Is(err, gerrors.ErrUnauthorized) {
		t.Errorf("mismatched issuer: err = %v, want %v", err, gerrors.ErrUnauthorized)
	}
}

// TestJWTVerifyAudience 配置 audience 后应校验 aud（支持字符串与数组两种形态）
func TestJWTVerifyAudience(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{Audience: "gateway"})

	asString := baseClaims(time.Now().Add(time.Hour))
	asString["aud"] = "gateway"
	if _, err := v.Verify(signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, asString)); err != nil {
		t.Errorf("string audience: unexpected err: %v", err)
	}

	asArray := baseClaims(time.Now().Add(time.Hour))
	asArray["aud"] = []any{"other", "gateway"}
	if _, err := v.Verify(signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, asArray)); err != nil {
		t.Errorf("array audience: unexpected err: %v", err)
	}

	asArray["aud"] = []any{"other"}
	if _, err := v.Verify(signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, asArray)); !errors.Is(err, gerrors.ErrUnauthorized) {
		t.Errorf("mismatched audience: err = %v, want %v", err, gerrors.ErrUnauthorized)
	}
}

// TestJWTVerifyLayoutKeycloak 按 iss 识别 Keycloak：角色取自嵌套的 realm_access.roles
func TestJWTVerifyLayoutKeycloak(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{})

	claims := baseClaims(time.Now().Add(time.Hour))
	claims["iss"] = "https://kc.example.com/realms/acme"
	claims["realm_access"] = map[string]any{"roles": []any{"admin", "ops"}}
	claims["roles"] = []any{"ignored"}
	token := signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, claims)

	got, err := v.Verify(token)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !slices.Equal(got.Roles, []string{"admin", "ops"}) {
		t.Errorf("Roles = %v, want [admin ops]", got.Roles)
	}
}

// TestJWTVerifyLayoutAzure 按 iss 识别 Azure：权限取自 scp，角色取自顶层 roles
func TestJWTVerifyLayoutAzure(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{})

	claims := baseClaims(time.Now().Add(time.Hour))
	claims["iss"] = "https://login.microsoftonline.com/tenant/v2.0"
	claims["roles"] = []any{"Reader"}
	claims["scp"] = "orders.read orders.write"
	token := signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, claims)

	got, err := v.Verify(token)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !slices.Equal(got.Roles, []string{"Reader"}) {
		t.Errorf("Roles = %v, want [Reader]", got.Roles)
	}
	if !slices.Equal(got.Scopes, []string{"orders.read", "orders.write"}) {
		t.Errorf("Scopes = %v, want [orders.read orders.write]", got.Scopes)
	}
}

// TestJWTVerifyLayoutGeneric 未识别的 issuer 回退通用布局：顶层 roles + scope
func TestJWTVerifyLayoutGeneric(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{})

	claims := baseClaims(time.Now().Add(time.Hour))
	claims["iss"] = "https://idp.example.com"
	claims["roles"] = []any{"admin"}
	claims["scope"] = "orders:read"
	token := signJWT(t, []byte(testSecret), map[string]any{"alg": "HS256"}, claims)

	got, err := v.Verify(token)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !slices.Equal(got.Roles, []string{"admin"}) {
		t.Errorf("Roles = %v, want [admin]", got.Roles)
	}
	if !slices.Equal(got.Scopes, []string{"orders:read"}) {
		t.Errorf("Scopes = %v, want [orders:read]", got.Scopes)
	}
}

// TestJWTVerifyMalformed 结构损坏的 token 应被拒绝
func TestJWTVerifyMalformed(t *testing.T) {
	v := newVerifier(t, builder.JWTVerifierConfig{})

	for _, token := range []string{"", "abc", "a.b", "a.b.c.d"} {
		if _, err := v.Verify(token); !errors.Is(err, gerrors.ErrUnauthorized) {
			t.Errorf("token %q: err = %v, want %v", token, err, gerrors.ErrUnauthorized)
		}
	}
}

package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"API_Gateway/builder"
	verifier "API_Gateway/handler/http_filter/auth/verifier"
	gconst "API_Gateway/pkg/constant"
	gerrors "API_Gateway/pkg/errors"
	"API_Gateway/pkg/tools"
)

type JWTVerifier struct {
	secret   []byte
	issuer   string
	audience string
	leeway   time.Duration

	now func() time.Time // 便于测试注入时钟
}

// NewJWTVerifier 从配置构造校验器：解析声明式密钥、做长度校验。
func NewJWTVerifier(cfg builder.JWTVerifierConfig) (*JWTVerifier, error) {
	secret := []byte(os.Getenv(cfg.SecretEnv))
	if len(secret) < gconst.AuthMinHS256SecretLen {
		return nil, fmt.Errorf("%w: secret env %q missing or shorter than %d bytes",
			gerrors.ErrInvalidAuthConfig, cfg.SecretEnv, gconst.AuthMinHS256SecretLen)
	}
	return &JWTVerifier{
		secret:   secret,
		issuer:   cfg.Issuer,
		audience: cfg.Audience,
		leeway:   cfg.Leeway,
		now:      time.Now,
	}, nil
}

// Verify 校验 HS256 JWT，按 iss 识别 IdP 并套用其布局归一化为 Verified
func (v *JWTVerifier) Verify(token string) (*verifier.Verified, error) {
	header, claims, signingInput, signature, err := splitJWT(token)
	if err != nil {
		return nil, err
	}

	if alg, _ := header["alg"].(string); alg != gconst.AuthAlgorithmHS256 {
		return nil, fmt.Errorf("%w: unsupported alg %q", gerrors.ErrUnauthorized, alg)
	}
	if !v.signatureValid(signingInput, signature) {
		return nil, fmt.Errorf("%w: signature mismatch", gerrors.ErrUnauthorized)
	}
	if err := v.checkTime(claims); err != nil {
		return nil, err
	}
	if err := v.checkIssuer(claims); err != nil {
		return nil, err
	}
	if err := v.checkAudience(claims); err != nil {
		return nil, err
	}
	return resolveLayout(claims).extract(claims), nil
}

// signatureValid 用 HMAC-SHA256 常数时间比对签名
func (v *JWTVerifier) signatureValid(signingInput string, signature []byte) bool {
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(signingInput))
	return hmac.Equal(mac.Sum(nil), signature)
}

// checkTime 校验 exp（必须存在）与 nbf，均按 leeway 放宽
func (v *JWTVerifier) checkTime(claims map[string]any) error {
	now := v.now()

	exp, ok := tools.ToUnixTime(claims["exp"])
	if !ok {
		return fmt.Errorf("%w: missing exp", gerrors.ErrUnauthorized)
	}
	if now.After(exp.Add(v.leeway)) {
		return fmt.Errorf("%w: token expired", gerrors.ErrUnauthorized)
	}

	if nbf, ok := tools.ToUnixTime(claims["nbf"]); ok && now.Add(v.leeway).Before(nbf) {
		return fmt.Errorf("%w: token not yet valid", gerrors.ErrUnauthorized)
	}
	return nil
}

// checkIssuer 配置了 issuer 时要求 iss 匹配
func (v *JWTVerifier) checkIssuer(claims map[string]any) error {
	if v.issuer == "" {
		return nil
	}
	if iss, _ := claims["iss"].(string); iss != v.issuer {
		return fmt.Errorf("%w: issuer mismatch", gerrors.ErrUnauthorized)
	}
	return nil
}

// checkAudience 配置了 audience 时要求 aud（字符串或数组）包含之
func (v *JWTVerifier) checkAudience(claims map[string]any) error {
	if v.audience == "" {
		return nil
	}
	for _, a := range tools.ToStringsLenient(claims["aud"]) {
		if a == v.audience {
			return nil
		}
	}
	return fmt.Errorf("%w: audience mismatch", gerrors.ErrUnauthorized)
}

// splitJWT 拆出 header/claims/signingInput/signature，任一环节损坏都返回包装 ErrUnauthorized 的错误
func splitJWT(token string) (header, claims map[string]any, signingInput string, signature []byte, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, "", nil, fmt.Errorf("%w: malformed token", gerrors.ErrUnauthorized)
	}

	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("%w: bad header encoding", gerrors.ErrUnauthorized)
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("%w: bad claims encoding", gerrors.ErrUnauthorized)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("%w: bad signature encoding", gerrors.ErrUnauthorized)
	}
	if err = json.Unmarshal(headerRaw, &header); err != nil {
		return nil, nil, "", nil, fmt.Errorf("%w: bad header json", gerrors.ErrUnauthorized)
	}
	if err = json.Unmarshal(claimsRaw, &claims); err != nil {
		return nil, nil, "", nil, fmt.Errorf("%w: bad claims json", gerrors.ErrUnauthorized)
	}
	return header, claims, parts[0] + "." + parts[1], sig, nil
}

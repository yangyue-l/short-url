package jwt

import (
	"errors"
	"short-url/settings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AccessTokenExpireDuration  = time.Minute * 15
	RefreshTokenExpireDuration = time.Hour * 24 * 7
)

// getSecret 从全局配置读取 JWT 签名密钥，配置未加载或密钥为空则 panic
func getSecret() []byte {
	if settings.Cfg == nil || settings.Cfg.JWT.Secret == "" {
		panic("JWT secret not configured: please set jwt.secret in config.yaml")
	}
	return []byte(settings.Cfg.JWT.Secret)
}

type MyClaims struct {
	UserID   int64  `json:"userId,string"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenToken 生成 Token（返回 access token 和 refresh token）
func GenToken(userID int64, userName, role string) (aToken, rToken string, err error) {
	accessClaims := MyClaims{
		UserID:   userID,
		Username: userName,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenExpireDuration)),
			Issuer:    "shorturl",
		},
	}
	aToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(getSecret())
	if err != nil {
		return "", "", err
	}

	refreshClaims := MyClaims{
		UserID:   userID,
		Username: userName,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(RefreshTokenExpireDuration)),
			Issuer:    "shorturl",
		},
	}
	rToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(getSecret())
	return
}

// ParseToken 解析 Token（严格校验过期时间，用于业务接口认证）
func ParseToken(tokenString string) (*MyClaims, error) {
	var mc = new(MyClaims)
	token, err := jwt.ParseWithClaims(tokenString, mc, func(token *jwt.Token) (any, error) {
		return getSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	if token.Valid {
		return mc, nil
	}
	return nil, errors.New("invalid token")
}

// ParseTokenForRefresh 解析 Token 用于刷新（允许已过期的 access token，最长 7 天窗口）
func ParseTokenForRefresh(tokenString string) (*MyClaims, error) {
	var mc = new(MyClaims)
	token, err := jwt.ParseWithClaims(tokenString, mc, func(token *jwt.Token) (any, error) {
		return getSecret(), nil
	}, jwt.WithLeeway(RefreshTokenExpireDuration))
	if err != nil {
		return nil, err
	}
	if token.Valid {
		return mc, nil
	}
	return nil, errors.New("invalid token")
}

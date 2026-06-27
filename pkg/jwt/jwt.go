package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AccessTokenExpireDuration  = time.Minute * 15
	RefreshTokenExpireDuration = time.Hour * 24 * 7
)

var mySecret = []byte("yangyue")

type MyClaims struct {
	UserID   int64  `json:"userId,string"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenToken 生成 Token（返回 access token 和 refresh token）
func GenToken(userID int64, userName string) (aToken, rToken string, err error) {
	accessClaims := MyClaims{
		UserID:   userID,
		Username: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenExpireDuration)),
			Issuer:    "shorturl",
		},
	}
	aToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(mySecret)
	if err != nil {
		return "", "", err
	}

	refreshClaims := MyClaims{
		UserID:   userID,
		Username: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(RefreshTokenExpireDuration)),
			Issuer:    "shorturl",
		},
	}
	rToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(mySecret)
	return
}

// ParseToken 解析 Token（严格校验过期时间，用于业务接口认证）
func ParseToken(tokenString string) (*MyClaims, error) {
	var mc = new(MyClaims)
	token, err := jwt.ParseWithClaims(tokenString, mc, func(token *jwt.Token) (any, error) {
		return mySecret, nil
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
		return mySecret, nil
	}, jwt.WithLeeway(RefreshTokenExpireDuration))
	if err != nil {
		return nil, err
	}
	if token.Valid {
		return mc, nil
	}
	return nil, errors.New("invalid token")
}

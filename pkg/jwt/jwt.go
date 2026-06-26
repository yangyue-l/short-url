package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const TokenExpireDuration = time.Hour * 2

var mySecret = []byte("yangyue")

type MyClaims struct {
	UserID   uint64 `json:"userId,string"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenToken 生成 Token
func GenToken(userID uint64, userName string) (string, error) {
	c := MyClaims{
		UserID:   userID,
		Username: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenExpireDuration)),
			Issuer:    "shorturl",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	return token.SignedString(mySecret)
}

// ParseToken 解析 Token
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

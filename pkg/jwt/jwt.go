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

// GenToken 生成 Token
func GenToken(userID int64, userName string) (aToken, rToken string, err error) {
	c := MyClaims{
		UserID:   userID,
		Username: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenExpireDuration)),
			Issuer:    "shorturl",
		},
	}
	aToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(mySecret)
	r := MyClaims{
		UserID:   userID,
		Username: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(RefreshTokenExpireDuration)),
			Issuer:    "shorturl",
		},
	}
	rToken, err = jwt.NewWithClaims(jwt.SigningMethodES256, r).SignedString(mySecret)
	return
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

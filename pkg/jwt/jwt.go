package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const TokenExpireDuration = time.Hour * 2

var mySecret = []byte("yangyue")

type MyClaims struct {
	UserID   int64  `json:"userId,string"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenToken 生成token
func GenToken(userID int64, userName string) (string, error) {
	c := MyClaims{
		UserID:   userID,
		Username: userName,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenExpireDuration)), //过期时间
			Issuer:    "shorturl",
		},
	}
	//使用指定的签名方法创建签名对象
	token := jwt.NewWithClaims(jwt.SigningMethodES256, c)
	return token.SignedString(mySecret)
}

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

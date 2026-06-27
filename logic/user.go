package logic

import (
	"errors"
	"short-url/dao/mysql"
	"short-url/models"
	"short-url/pkg/jwt"
	"short-url/pkg/snowflake"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserLogin = errors.New("用户名或密码错误")
)

func UserRegister(p *models.ParamRegisterRequest) (*models.ParamRegisterResponse, error) {
	userID := snowflake.GenID()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(p.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		ID:        userID,
		Username:  p.Username,
		Password:  string(hashedPassword),
		Email:     p.Email,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := mysql.CreateUser(user); err != nil {
		return nil, err
	}

	resp := &models.ParamRegisterResponse{
		UserID:   userID,
		Username: p.Username,
	}

	return resp, nil
}

func UserLogin(p *models.ParamLoginRequest) (*models.ParamLoginResponse, error) {
	user, err := mysql.GetUserByUsername(p.Username)
	if err != nil {
		return nil, ErrUserLogin
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(p.Password)); err != nil {
		return nil, ErrUserLogin
	}
	aToken, rToken, err := jwt.GenToken(user.ID, user.Username)
	if err != nil {
		return nil, err
	}
	resp := &models.ParamLoginResponse{
		Token:        aToken,
		RefreshToken: rToken,
		ExpireAt:     time.Now().Add(jwt.AccessTokenExpireDuration).Format(time.RFC3339),
	}
	return resp, nil
}

func UserRefresh(tokenString string) (*models.ParamRefreshResponse, error) {
	mc, err := jwt.ParseTokenForRefresh(tokenString)
	if err != nil {
		return nil, err
	}
	aToken, rToken, err := jwt.GenToken(mc.UserID, mc.Username)
	if err != nil {
		return nil, err
	}
	return &models.ParamRefreshResponse{
		Token:        aToken,
		RefreshToken: rToken,
		ExpireAt:     time.Now().Add(jwt.AccessTokenExpireDuration).Format(time.RFC3339),
	}, nil
}

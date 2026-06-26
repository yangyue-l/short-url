package logic

import (
	"short-url/dao/mysql"
	"short-url/models"
	"short-url/pkg/jwt"
	"short-url/pkg/snowflake"
	"time"
)

func UserRegister(p *models.ParamRegisterRequest) (*models.ParamRegisterResponse, error) {
	userID := snowflake.GenID()

	user := &models.User{
		ID:        userID,
		Username:  p.Username,
		Password:  p.Password,
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

func UserLogin(p *models.ParamLoginRequest) (string, error) {
	user, err := mysql.GetUserByUsername(p.Username)
	if err != nil {
		return "", err
	}
	token, err := jwt.GenToken(user.ID, user.Username)
	if err != nil {
		return "", err
	}
	return token, nil
}

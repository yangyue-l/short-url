package controller

import (
	"errors"
	"short-url/logic"
	"short-url/models"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func UserRegisterHandler(c *gin.Context) {
	var p *models.ParamRegisterRequest
	if err := c.ShouldBindJSON(&p); err != nil {
		zap.L().Error("invalid params", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}
	resp, err := logic.UserRegister(p)
	if err != nil {
		zap.L().Error("logic.UserRegister(p) failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, resp)
}

func UserLoginHandler(c *gin.Context) {
	var p *models.ParamLoginRequest
	if err := c.ShouldBindJSON(&p); err != nil {
		zap.L().Error("invalid params", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}
	resp, err := logic.UserLogin(p)
	if err != nil {
		if errors.Is(err, logic.ErrUserLogin) {
			ResponseErrorWithMsg(c, CodeNeedLogin, logic.ErrUserLogin.Error())
		} else {
			zap.L().Error("logic.UserLogin(p) failed", zap.Error(err))
			ResponseError(c, CodeServerBusy)
		}
		return
	}
	ResponseSuccess(c, resp)
}

func UserRefreshHandler(c *gin.Context) {
	authHeader := c.Request.Header.Get("Authorization")
	if authHeader == "" {
		ResponseError(c, CodeNeedLogin)
		return
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if !(len(parts) == 2 && parts[0] == "Bearer") {
		ResponseError(c, CodeInvalidToken)
		return
	}

	resp, err := logic.UserRefresh(parts[1])
	if err != nil {
		ResponseError(c, CodeInvalidToken)
		return
	}
	ResponseSuccess(c, resp)
}

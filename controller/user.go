package controller

import (
	"short-url/logic"
	"short-url/models"

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
		zap.L().Error("logic.UserLogin(p) failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, resp)
}

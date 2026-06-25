package controller

import (
	"net/http"
	"short-url/logic"
	"short-url/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ShortenHandler 创建短链接
func ShortenHandler(c *gin.Context) {
	var req models.ShortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		zap.L().Warn("invalid params", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}

	resp, err := logic.CreateShortURL(req.LongURL, req.ExpireIn)
	if err != nil {
		zap.L().Error("create short url failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}

	ResponseSuccess(c, resp)
}

// RedirectHandler 重定向到原始链接
func RedirectHandler(c *gin.Context) {
	shortCode := c.Param("shortCode")
	if shortCode == "" {
		ResponseError(c, CodeInvalidParam)
		return
	}

	longURL, err := logic.GetLongURL(shortCode)
	if err != nil {
		zap.L().Warn("get long url failed", zap.String("shortCode", shortCode), zap.Error(err))
		ResponseError(c, CodeNotFound)
		return
	}

	c.Redirect(http.StatusMovedPermanently, longURL)
}

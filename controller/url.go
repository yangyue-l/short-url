package controller

import (
	"errors"
	"net/http"
	"short-url/logic"
	"short-url/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ShortenHandler 创建短链接
func ShortenHandler(c *gin.Context) {
	var req models.ParamShortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		zap.L().Warn("invalid params", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}

	userID, err := GetCurrentUser(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}

	resp, err := logic.CreateShortURL(userID, req.LongURL, req.CustomCode, req.ExpireIn)
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

// ShortenInfoHandler 查询短链接信息
func ShortenInfoHandler(c *gin.Context) {
	shortCode := c.Param("shortCode")
	if shortCode == "" {
		ResponseError(c, CodeInvalidParam)
		return
	}
	resp, err := logic.GetShortenInfo(shortCode)
	if err != nil {
		zap.L().Error("logic.GetShortenInfo failed", zap.Error(err))
		ResponseError(c, CodeNotFound)
		return
	}
	ResponseSuccess(c, resp)
}

// BatchShortenHandler 批量创建短链接
func BatchShortenHandler(c *gin.Context) {
	var p models.ParamBatchURLRequest
	if err := c.ShouldBindJSON(&p); err != nil {
		zap.L().Warn("invalid params", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}
	if len(p.URLs) == 0 || len(p.URLs) > 50 {
		zap.L().Warn("batch urls count out of range", zap.Int("count", len(p.URLs)))
		ResponseError(c, CodeInvalidParam)
		return
	}

	userID, err := GetCurrentUser(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}

	resp, err := logic.CreateBatchShortURL(userID, &p)
	if err != nil {
		if errors.Is(err, logic.ErrRequestAlreadyProcessed) {
			ResponseError(c, CodeRequestAlreadyProcessed)
			return
		}
		zap.L().Error("logic.CreateBatchShortURL failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, resp)
}

// UpdateLongURLHandler 更新短链接（需要登录 + 所有权校验）
func UpdateLongURLHandler(c *gin.Context) {
	var p models.ParamUpdateRequest
	if err := c.ShouldBindJSON(&p); err != nil {
		zap.L().Warn("invalid params", zap.Error(err))
		ResponseError(c, CodeInvalidParam)
		return
	}

	shortCode := c.Param("shortCode")
	userID, err := GetCurrentUser(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}

	resp, err := logic.UpdateLongURL(userID, shortCode, &p)
	if err != nil {
		zap.L().Error("logic.UpdateLongURL failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, resp)
}

func DeleteShortenHandler(c *gin.Context) {
	userID, err := GetCurrentUser(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	shortCode := c.Param("shortCode")
	if err := logic.DeleteShortURL(userID, shortCode); err != nil {
		zap.L().Error("logic.DeleteShortURL(userID, shortCode) failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, nil)
}

func GetURLsHandler(c *gin.Context) {
	userID, err := GetCurrentUser(c)
	if err != nil {
		ResponseError(c, CodeNeedLogin)
		return
	}
	page, pageSize := GetPageInfo(c)
	resp, err := logic.GetUserURLs(userID, int(page), int(pageSize))
	if err != nil {
		zap.L().Error("logic.GetUserURLs failed", zap.Error(err))
		ResponseError(c, CodeServerBusy)
		return
	}
	ResponseSuccess(c, resp)
}

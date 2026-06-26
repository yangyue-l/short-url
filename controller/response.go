package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ResponseCode int

const (
	CodeSuccess          ResponseCode = 1000 // 请求成功
	CodeInvalidParam     ResponseCode = 1001 // 参数错误
	CodeNotFound         ResponseCode = 1002 // 资源未找到
	CodeServerBusy       ResponseCode = 1003 // 服务器内部错误
	CodeNeedLogin        ResponseCode = 1004 // 未授权（需要登录）
	CodePermissionDenied ResponseCode = 1005 // 权限不足
	CodeResourceExists   ResponseCode = 1006 // 资源已存在
	CodeRateLimit        ResponseCode = 1007 // 请求频率过高

	// 内部扩展码
	CodeInvalidToken            ResponseCode = 1400 // 无效 Token
	CodeRequestAlreadyProcessed ResponseCode = 1401 // 幂等请求已处理
)

var codeMsg = map[ResponseCode]string{
	CodeSuccess:                 "success",
	CodeInvalidParam:            "请求参数错误",
	CodeNotFound:                "没有找到对应的参数",
	CodeServerBusy:              "服务繁忙",
	CodeNeedLogin:               "需要登录",
	CodePermissionDenied:        "权限不足",
	CodeResourceExists:          "资源已存在",
	CodeRateLimit:               "请求频率过高",
	CodeInvalidToken:            "无效的 Token",
	CodeRequestAlreadyProcessed: "请求已经处理",
}

type Response struct {
	Code    ResponseCode `json:"code"`
	Message string       `json:"message"`
	Data    any          `json:"data,omitempty"`
}

func ResponseSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: codeMsg[CodeSuccess],
		Data:    data,
	})
}

func ResponseError(c *gin.Context, code ResponseCode) {
	c.JSON(http.StatusOK, Response{
		Code:    code,
		Message: codeMsg[code],
	})
}

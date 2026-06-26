package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ResponseCode int

const (
	CodeSuccess ResponseCode = 1000 + iota
	CodeInvalidParam
	CodeNotFound
	CodeServerBusy
	CodeRequestAlreadyProcessed

	CodeNeedLogin
	CodeInvalidToken
)

var codeMsg = map[ResponseCode]string{
	CodeSuccess:                 "success",
	CodeInvalidParam:            "请求参数错误",
	CodeNotFound:                "没有找到对应的参数",
	CodeServerBusy:              "服务繁忙",
	CodeRequestAlreadyProcessed: "请求已经处理",
	CodeNeedLogin:               "需要登录",
	CodeInvalidToken:            "无效的token",
}

type Response struct {
	Code    ResponseCode `json:"code"`
	Message string       `json:"message"`
	Data    interface{}  `json:"data,omitempty"`
}

func ResponseSuccess(c *gin.Context, data interface{}) {
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

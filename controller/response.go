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
)

var codeMsg = map[ResponseCode]string{
	CodeSuccess:                 "success",
	CodeInvalidParam:            "invalid parameters",
	CodeNotFound:                "not found",
	CodeServerBusy:              "server busy",
	CodeRequestAlreadyProcessed: "request already processed",
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

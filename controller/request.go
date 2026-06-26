package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
)

const CtxUserIDKey = "userID"

var ErrorUserNotLogin = errors.New("用户未登录")

// GetCurrentUser 获取当前登录用户的ID
func GetCurrentUser(c *gin.Context) (int64, error) {
	uid, ok := c.Get(CtxUserIDKey)
	if !ok {
		return 0, ErrorUserNotLogin
	}
	userID, ok := uid.(int64)
	if !ok {
		return 0, ErrorUserNotLogin
	}
	return userID, nil
}

// GetPageInfo 从 query string 获取分页参数，默认 page=1, page_size=10
func GetPageInfo(c *gin.Context) (int64, int64) {
	pageStr := c.Query("page")
	sizeStr := c.Query("page_size")

	var (
		page int64
		size int64
		err  error
	)

	page, err = strconv.ParseInt(pageStr, 10, 64)
	if err != nil {
		page = 1
	}
	size, err = strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		size = 10
	}
	return page, size
}

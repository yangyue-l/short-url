package middlewares

import (
	"short-url/controller"
	"short-url/pkg/jwt"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTAuthMiddleware 强制认证中间件：无有效 Token 时拦截请求
func JWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.Request.Header.Get("Authorization")
		if authHeader == "" {
			controller.ResponseError(c, controller.CodeNeedLogin)
			c.Abort()
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			controller.ResponseError(c, controller.CodeInvalidToken)
			c.Abort()
			return
		}
		mc, err := jwt.ParseToken(parts[1])
		if err != nil {
			controller.ResponseError(c, controller.CodeInvalidToken)
			c.Abort()
			return
		}
		c.Set(controller.CtxUserIDKey, mc.UserID)
		c.Next()
	}
}

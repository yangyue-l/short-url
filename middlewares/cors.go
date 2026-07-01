package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// allowedOrigins CORS 白名单（生产环境需配置真实域名）
var allowedOrigins = map[string]bool{
	"http://localhost:3000":     true,
	"http://localhost:8080":     true,
	"https://your-frontend.com": true,
}

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")

		if origin == "" {
			c.Next()
			return
		}

		if allowedOrigins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

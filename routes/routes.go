package routes

import (
	"short-url/controller"
	"short-url/middlewares"

	"github.com/gin-gonic/gin"
)

func Setup(mode string) *gin.Engine {
	if mode == gin.ReleaseMode {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(middlewares.CORS())
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// 公开：无需登录
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})
	// 访问短链接
	r.GET("/:shortCode", controller.RedirectHandler)

	// 需要登录
	v1 := r.Group("/api/v1")
	v1.Use(middlewares.JWTAuthMiddleware())
	{
		//创建短链接
		v1.POST("/shorten", controller.ShortenHandler)
		// 获取短链接信息
		v1.GET("/:shortCode", controller.ShortenInfoHandler)
		// 批量创建短链接
		v1.POST("/batch/shorten", controller.BatchShortenHandler)
		// 修改目标链接信息
		v1.PUT("/:shortCode", controller.UpdateShortenHandler)
	}

	return r
}

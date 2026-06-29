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
	// 用户注册

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
		v1.PUT("/:shortCode", controller.UpdateLongURLHandler)
		// 删除目标链接
		v1.DELETE("/:shortCode", controller.DeleteShortenHandler)
		// 获取用户短链接
		v1.GET("/user/urls", controller.GetURLsHandler)
		// 用户短链接访问统计
		v1.GET("/:shortCode/stats", controller.GetShortStatsHandler)

		// 注销账户
		v1.DELETE("/user/account", controller.DeleteUserHandler)
	}

	v2 := r.Group("/api/v1")
	{
		v2.POST("/auth/register", controller.UserRegisterHandler)
		v2.POST("/auth/login", controller.UserLoginHandler)
		v2.POST("/auth/refresh", controller.UserRefreshHandler)
	}

	return r
}

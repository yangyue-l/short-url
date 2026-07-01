package routes

import (
	"context"
	"net/http"
	"short-url/controller"
	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/middlewares"
	"time"

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

	// ─── 限流策略（双层） ───
	//  ① IP 层（JWT 之前）：防 DDoS / 撞库，阈值宽松
	//  ② UserID 层（JWT 之后）：防单用户滥用，每个用户独立配额
	//
	//  跳转：RedirectRateLimit  (IP, 3000/min) — 仅防脚本刷量
	//  登录：LoginRateLimit     (IP, 5/min)    — 防撞库
	//  API： APIRateLimit       (IP, 300/min)  — 粗粒度防 DDoS
	//        UserRateLimit      (UserID, 200/min) — 细粒度按用户

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		checks := map[string]string{}
		if err := mysql.GetDB().WithContext(ctx).Raw("SELECT 1").Error; err != nil {
			checks["mysql"] = "unhealthy: " + err.Error()
		} else {
			checks["mysql"] = "healthy"
		}
		if err := redis.GetClient().Ping(ctx).Err(); err != nil {
			checks["redis"] = "unhealthy: " + err.Error()
		} else {
			checks["redis"] = "healthy"
		}

		httpStatus := http.StatusOK
		for _, v := range checks {
			if v != "healthy" {
				httpStatus = http.StatusServiceUnavailable
				break
			}
		}
		c.JSON(httpStatus, checks)
	})

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	// 访问短链接（公开接口，IP 维度高阈值限流）
	r.GET("/:shortCode", middlewares.RedirectRateLimit(), controller.RedirectHandler)

	// ─── 认证组（/api/v1/auth） ───
	auth := r.Group("/api/v1/auth")
	auth.Use(middlewares.APIRateLimit()) // ① IP 层
	{
		auth.POST("/register", controller.UserRegisterHandler)
		auth.POST("/login", middlewares.LoginRateLimit(), controller.UserLoginHandler) // 登录单独严格限流
		auth.POST("/refresh", controller.UserRefreshHandler)
	}

	// ─── API v1 组（需要登录） ───
	//  双层限流：IP 层（防 DDoS）→ JWT → UserID 层（防用户滥用）
	v1 := r.Group("/api/v1")
	v1.Use(middlewares.APIRateLimit())      // ① IP 层：防 DDoS（300/min）
	v1.Use(middlewares.JWTAuthMiddleware()) // ② JWT 认证
	v1.Use(middlewares.UserRateLimit())     // ③ UserID 层：按用户限流（200/min）
	{
		v1.POST("/shorten", controller.ShortenHandler)
		v1.GET("/:shortCode", controller.ShortenInfoHandler)
		v1.POST("/batch/shorten", controller.BatchShortenHandler)
		v1.PUT("/:shortCode", controller.UpdateLongURLHandler)
		v1.DELETE("/:shortCode", controller.DeleteShortenHandler)
		v1.GET("/user/urls", controller.GetURLsHandler)
		v1.GET("/:shortCode/stats", controller.GetShortStatsHandler)
		v1.DELETE("/user/account", controller.DeleteUserHandler)
	}

	// ─── 管理员接口 ───
	admin := r.Group("/api/v1")
	admin.Use(middlewares.APIRateLimit())                                     // ① IP 层
	admin.Use(middlewares.JWTAuthMiddleware())                                // ② JWT
	admin.Use(middlewares.AdminAuthMiddleware(), middlewares.UserRateLimit()) // ③ 管理员权限 + UserID 层
	{
		admin.GET("/urls", controller.GetAdminURLsHandler)
		admin.GET("/stats/overview", controller.GetStatsOverviewHandler)
	}

	return r
}

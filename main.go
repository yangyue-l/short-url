package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/logger"
	"short-url/logic"
	"short-url/routes"
	"short-url/settings"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func main() {
	// 1. 加载配置
	cfg, err := settings.Init("./config.yaml")
	if err != nil {
		panic(fmt.Sprintf("load config failed: %v", err))
	}

	// 2. 初始化日志
	if err := logger.Init(&cfg.Logger); err != nil {
		panic(fmt.Sprintf("init logger failed: %v", err))
	}
	defer zap.L().Sync()
	zap.L().Debug("logger init success")

	// 3. 初始化 MySQL
	if err := mysql.Init(&cfg.MySQL); err != nil {
		zap.L().Fatal("init mysql failed", zap.Error(err))
	}
	defer mysql.Close()

	// 4. 初始化 Redis
	if err := redis.Init(&cfg.Redis); err != nil {
		zap.L().Fatal("init redis failed", zap.Error(err))
	}
	defer redis.Close()

	// 4.1 启动点击计数 worker
	logic.InitClickWorkers(3)

	// 5. 注册路由
	r := routes.Setup(cfg.Server.Mode)

	// 6. 启动服务（支持优雅关闭）
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		zap.L().Info("server starting", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("server start failed", zap.Error(err))
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	zap.L().Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Fatal("server shutdown failed", zap.Error(err))
	}
	log.Println("server exited")
}

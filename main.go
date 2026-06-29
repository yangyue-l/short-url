package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"short-url/cron"
	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/logger"
	"short-url/mq"
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

	// 5. 初始化 RabbitMQ 连接池
	if err := mq.Init(&cfg.RabbitMQ); err != nil {
		zap.L().Fatal("init RabbitMQ failed", zap.Error(err))
	}
	defer mq.Close()

	// 6. 启动 MQ 消费者（批量聚合 + 写 Redis）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mq.StartConsumers(ctx, cfg.RabbitMQ.Consumer.Workers)

	// 7. 启动定时回刷任务（Redis → MySQL）
	cron.StartFlushJob(ctx)

	// 8. 注册路由
	r := routes.Setup(cfg.Server.Mode)

	// 9. 启动服务（支持优雅关闭）
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

	// 先停 HTTP
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		zap.L().Fatal("server shutdown failed", zap.Error(err))
	}

	// 停止消费者 + 回刷
	cancel()
	mq.StopConsumers()

	log.Println("server exited")
}

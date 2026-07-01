package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"short-url/cron"
	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/logger"
	"short-url/logic"
	"short-url/middlewares"
	"short-url/mq"
	"short-url/pkg/snowflake"
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

	// 3. 初始化雪花 ID 生成器
	if err := snowflake.Init(cfg.Snowflake.StartTime, cfg.Snowflake.MachineID); err != nil {
		zap.L().Fatal("init snowflake failed", zap.Error(err))
	}

	// 4. 初始化 MySQL
	if err := mysql.Init(&cfg.MySQL); err != nil {
		zap.L().Fatal("init mysql failed", zap.Error(err))
	}
	defer mysql.Close()

	// 5. 初始化 Redis
	if err := redis.Init(&cfg.Redis); err != nil {
		zap.L().Fatal("init redis failed", zap.Error(err))
	}
	defer redis.Close()

	// 6. 初始化 RabbitMQ 连接池
	if err := mq.Init(&cfg.RabbitMQ); err != nil {
		zap.L().Fatal("init RabbitMQ failed", zap.Error(err))
	}
	defer mq.Close()

	// 6.5 初始化布隆过滤器并加载已有短码（防缓存穿透）
	logic.InitBloomFilter(10_000_000, 0.001) // 1000万条，0.1%误判率
	if codes, err := mysql.GetAllShortCodes(); err != nil {
		zap.L().Warn("load short codes for bloom filter failed", zap.Error(err))
	} else {
		logic.LoadBloomFromDB(codes)
	}

	// 7. 启动 MQ 消费者（批量聚合 + 写 Redis）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mq.StartConsumers(ctx, cfg.RabbitMQ.Consumer.Workers)

	// 8. 启动定时回刷任务（Redis → MySQL）
	cron.StartFlushJob(ctx)

	// 8.5 启动日志清理任务（每天凌晨 3 点清理 90 天前日志）
	cron.StartCleanupJob(ctx)

	// 9. 注册路由
	r := routes.Setup(cfg.Server.Mode)

	// 10. 启动服务（支持优雅关闭 + 超时防 Slowloris）
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,  // 读取请求的超时（含 body）
		WriteTimeout: 30 * time.Second,  // 写响应的超时
		IdleTimeout:  120 * time.Second, // keep-alive 空闲超时
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

	// 停止消费者 + 限流器清理协程 + 回刷
	cancel()
	middlewares.StopLimiters()
	mq.StopConsumers()

	zap.L().Info("server exited")
}

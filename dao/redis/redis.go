package redis

import (
	"context"
	"fmt"
	"short-url/settings"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

var rdb *redis.Client
var ctx = context.Background()

func Init(cfg *settings.RedisConfig) error {
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	}
	if cfg.MinIdleConns > 0 {
		opts.MinIdleConns = cfg.MinIdleConns
	}
	if cfg.PoolTimeoutSec > 0 {
		opts.PoolTimeout = time.Duration(cfg.PoolTimeoutSec) * time.Second
	}
	if cfg.ReadTimeoutMillis > 0 {
		opts.ReadTimeout = time.Duration(cfg.ReadTimeoutMillis) * time.Millisecond
	}
	if cfg.WriteTimeoutMillis > 0 {
		opts.WriteTimeout = time.Duration(cfg.WriteTimeoutMillis) * time.Millisecond
	}
	rdb = redis.NewClient(opts)

	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("connect redis failed: %w", err)
	}

	zap.L().Info("redis init success")
	return nil
}

func Close() {
	if rdb != nil {
		rdb.Close()
	}
}

func GetClient() *redis.Client {
	return rdb
}

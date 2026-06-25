package redis

import (
	"context"
	"fmt"
	"short-url/settings"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

var rdb *redis.Client
var ctx = context.Background()

func Init(cfg *settings.RedisConfig) error {
	rdb = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

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

package cron

import (
	"context"
	"math"
	"time"

	"short-url/dao/mysql"
	"short-url/dao/redis"

	"go.uber.org/zap"
)

const (
	flushInterval   = 5 * time.Minute
	activeWindowSec = 60 * 60
)

// StartFlushJob 启动定时回刷任务
func StartFlushJob(ctx context.Context) {
	zap.L().Info("cron flush job started", zap.Duration("interval", flushInterval))

	go func() {
		time.Sleep(30 * time.Second)

		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				zap.L().Info("cron flush job stopping, final flush...")
				flushOnce()
				return
			case <-ticker.C:
				flushOnce()
			}
		}
	}()
}

func flushOnce() {
	client := redis.GetClient()
	if client == nil {
		return
	}

	since := time.Now().Unix() - activeWindowSec
	codes, err := redis.GetActiveShortCodes(client, since)
	if err != nil {
		zap.L().Error("get active short codes failed", zap.Error(err))
		return
	}

	if len(codes) == 0 {
		return
	}

	zap.L().Info("flushing active short codes to MySQL", zap.Int("count", len(codes)))

	batchSize := 50
	flushed := 0
	for i := 0; i < len(codes); i += batchSize {
		end := int(math.Min(float64(i+batchSize), float64(len(codes))))
		batch := codes[i:end]
		flushBatch(batch)
		flushed += len(batch)
	}

	zap.L().Info("flush completed", zap.Int("flushed", flushed))
}

func flushBatch(codes []string) {
	client := redis.GetClient()

	// 第一步：读取 Redis PV 增量（不重置，防数据丢失）
	pvMap := make(map[string]int64)
	for _, code := range codes {
		delta := redis.GetPV(client, code)
		if delta > 0 {
			pvMap[code] = delta
		}
	}

	// 第二步：先写 MySQL，成功后再重置 Redis
	if len(pvMap) > 0 {
		if err := mysql.IncrementClickCntBatch(pvMap); err != nil {
			zap.L().Error("increment click cnt batch failed, retry next flush", zap.Error(err))
			return // MySQL 失败保留 Redis 数据，下次重试
		}
	}

	// 第三步：MySQL 成功后重置 Redis PV 计数
	for code := range pvMap {
		redis.ResetPV(client, code)
	}
	redis.CleanActive(client, codes...)

	if len(pvMap) > 0 {
		zap.L().Info("flush batch completed",
			zap.Int("codes", len(codes)),
			zap.Int("updated", len(pvMap)))
	}
}

// FlushPV 显式回刷单个短链（供 API 调用，先写 MySQL 再重置 Redis）
func FlushPV(shortCode string) int64 {
	client := redis.GetClient()
	if client == nil {
		return 0
	}

	delta := redis.GetPV(client, shortCode)
	if delta > 0 {
		if err := mysql.IncrementClickCntBatch(map[string]int64{shortCode: delta}); err != nil {
			zap.L().Error("flush single pv failed", zap.String("shortCode", shortCode), zap.Error(err))
			return delta // Redis 保留，下次重试
		}
		redis.ResetPV(client, shortCode)
	}
	redis.CleanActive(client, shortCode)
	return delta
}

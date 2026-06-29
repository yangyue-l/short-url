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

	pvMap := make(map[string]int64)
	var toClean []string

	for _, code := range codes {
		delta := redis.FlushPV(client, code)
		if delta > 0 {
			pvMap[code] = delta
			toClean = append(toClean, code)
		} else {
			toClean = append(toClean, code)
		}
	}

	if len(pvMap) > 0 {
		if err := mysql.IncrementClickCntBatch(pvMap); err != nil {
			zap.L().Error("increment click cnt batch failed", zap.Error(err))
		}
	}

	if len(toClean) > 0 {
		redis.CleanActive(client, toClean...)
	}
}

// FlushPV 显式回刷单个短链（供 API 调用）
func FlushPV(shortCode string) int64 {
	client := redis.GetClient()
	if client == nil {
		return 0
	}

	delta := redis.FlushPV(client, shortCode)
	if delta > 0 {
		mysql.IncrementClickCntBatch(map[string]int64{shortCode: delta})
		redis.CleanActive(client, shortCode)
	}
	return delta
}

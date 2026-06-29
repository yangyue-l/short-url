package mq

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/models"

	"go.uber.org/zap"
)

const (
	batchSize   = 100
	flushPeriod = 5 * time.Second
)

// batchBuffer 批量缓冲区
type batchBuffer struct {
	mu        sync.Mutex
	clicks    []*models.ClickItem
	lastFlush time.Time
}

var (
	buf       = &batchBuffer{lastFlush: time.Now()}
	bufCtx    context.Context
	bufCancel context.CancelFunc
)

// StartConsumers 启动多个消费者协程
func StartConsumers(ctx context.Context, workerNum int) {
	bufCtx, bufCancel = context.WithCancel(ctx)

	pool := GetPool()
	if pool == nil {
		zap.L().Fatal("MQ pool not initialized, cannot start consumers")
	}

	go batchFlushLoop()

	for i := 0; i < workerNum; i++ {
		go consumeWorker(i)
	}
	zap.L().Info("MQ consumers started", zap.Int("workers", workerNum))
}

func consumeWorker(id int) {
	for {
		select {
		case <-bufCtx.Done():
			return
		default:
		}

		pool := GetPool()
		if pool == nil {
			time.Sleep(3 * time.Second)
			continue
		}

		msgs, err := pool.Consume()
		if err != nil {
			zap.L().Error("consume worker start failed", zap.Int("worker", id), zap.Error(err))
			time.Sleep(3 * time.Second)
			continue
		}

		zap.L().Info("consume worker started", zap.Int("worker", id))

		for {
			select {
			case <-bufCtx.Done():
				return
			case msg, ok := <-msgs:
				if !ok {
					zap.L().Warn("consume channel closed, reconnecting...", zap.Int("worker", id))
					goto reconnect
				}

				var event ClickEvent
				if err := json.Unmarshal(msg.Body, &event); err != nil {
					zap.L().Warn("unmarshal click event failed", zap.Error(err))
					msg.Ack(false)
					continue
				}

				buf.mu.Lock()
				buf.clicks = append(buf.clicks, &models.ClickItem{
					ShortCode: event.ShortCode,
					IP:        event.IP,
					Referer:   event.Referer,
					UserAgent: event.UserAgent,
					Timestamp: event.Timestamp,
				})
				buf.mu.Unlock()

				msg.Ack(false)

				buf.mu.Lock()
				shouldFlush := len(buf.clicks) >= batchSize
				buf.mu.Unlock()
				if shouldFlush {
					flushBatch()
				}
			}
		}
	reconnect:
		time.Sleep(2 * time.Second)
	}
}

// batchFlushLoop 定时刷盘循环
func batchFlushLoop() {
	ticker := time.NewTicker(flushPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-bufCtx.Done():
			flushBatch()
			return
		case <-ticker.C:
			flushBatch()
		}
	}
}

// flushBatch 批量刷盘：写 Redis 计数 + 批量写 MySQL 日志
func flushBatch() {
	buf.mu.Lock()
	if len(buf.clicks) == 0 {
		buf.mu.Unlock()
		return
	}
	clicks := buf.clicks
	buf.clicks = make([]*models.ClickItem, 0, batchSize)
	buf.lastFlush = time.Now()
	buf.mu.Unlock()

	pvMap := make(map[string]int64)
	for _, c := range clicks {
		pvMap[c.ShortCode]++
	}

	redis.BatchIncrPV(redis.GetClient(), pvMap)
	redis.BatchAddUV(redis.GetClient(), clicks)
	redis.MarkActive(redis.GetClient(), pvMap)

	go func() {
		if err := mysql.BatchCreateClickLogs(clicks); err != nil {
			zap.L().Error("batch create click logs failed", zap.Error(err))
		}
	}()

	zap.L().Debug("batch flushed",
		zap.Int("count", len(clicks)),
		zap.Int("shortCodes", len(pvMap)))
}

// StopConsumers 优雅关闭消费者
func StopConsumers() {
	if bufCancel != nil {
		bufCancel()
	}
	zap.L().Info("MQ consumers stopped")
}

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

// batchBuffer 批量缓冲区（仅用于异步批量写 MySQL）
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
	reconnectDelay := 2 * time.Second

	for {
		select {
		case <-bufCtx.Done():
			return
		default:
		}

		pool := GetPool()
		if pool == nil {
			select {
			case <-bufCtx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}

		msgs, err := pool.Consume()
		if err != nil {
			zap.L().Error("consume worker start failed", zap.Int("worker", id), zap.Error(err))
			select {
			case <-bufCtx.Done():
				return
			case <-time.After(reconnectDelay):
			}
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
					select {
					case <-bufCtx.Done():
						return
					case <-time.After(reconnectDelay):
					}
					goto reconnect
				}

				var event ClickEvent
				if err := json.Unmarshal(msg.Body, &event); err != nil {
					zap.L().Warn("unmarshal click event failed", zap.Error(err))
					msg.Nack(false, false) // 格式错误不重试，进入 DLQ
					continue
				}

				item := &models.ClickItem{
					ShortCode: event.ShortCode,
					IP:        event.IP,
					Referer:   event.Referer,
					UserAgent: event.UserAgent,
					Timestamp: event.Timestamp,
				}

				// ★ 关键修复：先写 Redis（每条消息立即持久化），再 ACK
				// 保证 at-least-once 语义：即使进程在 MySQL 写入前崩溃，Redis 中有数据
				client := redis.GetClient()
				redis.BatchIncrPV(client, map[string]int64{event.ShortCode: 1})
				redis.BatchAddUV(client, []*models.ClickItem{item})
				redis.MarkActive(client, map[string]int64{event.ShortCode: 1})
				redis.IncrTodayClick(client, 1)

				// Redis 写入成功后 ACK
				msg.Ack(false)

				// 追加到缓冲区（仅用于异步批量写 MySQL + 预聚合）
				buf.mu.Lock()
				buf.clicks = append(buf.clicks, item)
				n := len(buf.clicks)
				buf.mu.Unlock()

				if n >= batchSize {
					flushBatch()
				}
			}
		}
	reconnect:
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

// flushBatch 批量写 MySQL 日志 + 每日预聚合（Redis 已在消费时实时写入）
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

	// 按短码聚合 PV/UV（用于预聚合表）
	pvMap := make(map[string]int64)
	uvMap := make(map[string]map[string]struct{})
	for _, c := range clicks {
		pvMap[c.ShortCode]++
		if uvMap[c.ShortCode] == nil {
			uvMap[c.ShortCode] = make(map[string]struct{})
		}
		uvMap[c.ShortCode][c.IP] = struct{}{}
	}

	// 异步批量写 MySQL 日志
	go func() {
		if err := mysql.BatchCreateClickLogs(clicks); err != nil {
			zap.L().Error("batch create click logs failed", zap.Error(err))
		}
	}()

	// 异步更新每日预聚合统计表
	go func() {
		today := time.Now().Format("2006-01-02")
		for code, pv := range pvMap {
			uv := int64(len(uvMap[code]))
			if err := mysql.UpsertClickStatsDaily(today, code, pv, uv); err != nil {
				zap.L().Warn("upsert click stats daily failed",
					zap.String("shortCode", code), zap.Error(err))
			}
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

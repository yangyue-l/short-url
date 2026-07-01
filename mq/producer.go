package mq

import (
	"encoding/json"
	"time"

	"short-url/dao/redis"
	"short-url/models"
	"short-url/settings"

	"go.uber.org/zap"
)

// ClickEvent 点击事件消息体
type ClickEvent struct {
	ShortCode string `json:"short_code"`
	IP        string `json:"ip"`
	Referer   string `json:"referer"`
	UserAgent string `json:"user_agent"`
	Timestamp int64  `json:"timestamp"`
}

var defaultPool *ConnPool

// Init 初始化全局连接池
func Init(cfg *settings.RabbitMQConfig) error {
	pool, err := NewConnPool(cfg)
	if err != nil {
		return err
	}
	defaultPool = pool
	return nil
}

// PublishClick 投递点击事件到队列，MQ 不可用时降级直接写 Redis
func PublishClick(event *ClickEvent) {
	body, err := json.Marshal(event)
	if err != nil {
		zap.L().Error("marshal click event failed", zap.Error(err))
		return
	}

	if defaultPool == nil {
		zap.L().Warn("MQ pool not initialized, fallback to direct Redis")
		fallbackToRedis(event)
		return
	}

	if err := defaultPool.Publish(body); err != nil {
		zap.L().Warn("publish click event failed, fallback to direct Redis",
			zap.String("shortCode", event.ShortCode),
			zap.Error(err))
		fallbackToRedis(event)
	}
}

// fallbackToRedis MQ 不可用时的降级方案：直接写 Redis 计数
func fallbackToRedis(event *ClickEvent) {
	client := redis.GetClient()
	if client == nil {
		return
	}
	item := &models.ClickItem{
		ShortCode: event.ShortCode,
		IP:        event.IP,
		Referer:   event.Referer,
		UserAgent: event.UserAgent,
		Timestamp: event.Timestamp,
	}
	redis.BatchIncrPV(client, map[string]int64{event.ShortCode: 1})
	redis.BatchAddUV(client, []*models.ClickItem{item})
	redis.MarkActive(client, map[string]int64{event.ShortCode: 1})
	redis.IncrTodayClick(client, 1)
}

// Close 关闭全局连接池
func Close() {
	if defaultPool != nil {
		defaultPool.Close()
	}
}

// GetPool 返回全局连接池（供消费者使用）
func GetPool() *ConnPool {
	return defaultPool
}

// RecordClick 异步记录点击（供 controller 调用）
func RecordClick(shortCode, ip, referer, userAgent string) {
	PublishClick(&ClickEvent{
		ShortCode: shortCode,
		IP:        ip,
		Referer:   referer,
		UserAgent: userAgent,
		Timestamp: time.Now().UnixMilli(),
	})
}

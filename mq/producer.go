package mq

import (
	"encoding/json"
	"time"

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

// PublishClick 投递点击事件到队列
func PublishClick(event *ClickEvent) {
	if defaultPool == nil {
		zap.L().Error("MQ pool not initialized")
		return
	}

	body, err := json.Marshal(event)
	if err != nil {
		zap.L().Error("marshal click event failed", zap.Error(err))
		return
	}

	if err := defaultPool.Publish(body); err != nil {
		zap.L().Error("publish click event failed",
			zap.String("shortCode", event.ShortCode),
			zap.Error(err))
	}
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

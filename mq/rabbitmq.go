package mq

import (
	"fmt"
	"sync"
	"time"

	"short-url/settings"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// ConnPool RabbitMQ 连接池，维护一组 Channel，支持自动重连
type ConnPool struct {
	cfg      *settings.RabbitMQConfig
	mu       sync.Mutex
	conn     *amqp.Connection
	channels []*amqp.Channel
	closed   chan struct{}
	done     chan struct{}
	failCnt  int // 连续重连失败计数
}

// NewConnPool 创建 RabbitMQ 连接池
func NewConnPool(cfg *settings.RabbitMQConfig) (*ConnPool, error) {
	pool := &ConnPool{
		cfg:    cfg,
		closed: make(chan struct{}),
		done:   make(chan struct{}),
	}

	if err := pool.connect(); err != nil {
		return nil, err
	}

	go pool.reconnectLoop()
	return pool, nil
}

// connect 建立连接 + 申明交换机/队列 + 创建 Channel 池
func (p *ConnPool) connect() error {
	conn, err := amqp.Dial(p.cfg.Addr())
	if err != nil {
		return fmt.Errorf("dial RabbitMQ failed: %w", err)
	}

	size := p.cfg.Consumer.Workers + 1 // 消费者 worker 数 + 1 个给生产者
	channels := make([]*amqp.Channel, 0, size)
	for i := 0; i < size; i++ {
		ch, err := conn.Channel()
		if err != nil {
			conn.Close()
			return fmt.Errorf("create channel #%d failed: %w", i, err)
		}
		// 申明交换机
		if err := ch.ExchangeDeclare(
			p.cfg.Click.Exchange,
			p.cfg.Click.ExchangeType,
			p.cfg.Click.Durable,
			p.cfg.Click.AutoDelete,
			false,
			false,
			nil,
		); err != nil {
			conn.Close()
			return fmt.Errorf("declare exchange failed: %w", err)
		}
		// 申明队列
		if _, err := ch.QueueDeclare(
			p.cfg.Click.Queue,
			p.cfg.Click.Durable,
			p.cfg.Click.AutoDelete,
			false,
			false,
			nil,
		); err != nil {
			conn.Close()
			return fmt.Errorf("declare queue failed: %w", err)
		}
		// 绑定队列到交换机
		if err := ch.QueueBind(
			p.cfg.Click.Queue,
			p.cfg.Click.RoutingKey,
			p.cfg.Click.Exchange,
			false,
			nil,
		); err != nil {
			conn.Close()
			return fmt.Errorf("bind queue failed: %w", err)
		}
		channels = append(channels, ch)
	}

	p.mu.Lock()
	p.conn = conn
	p.channels = channels
	p.failCnt = 0
	p.mu.Unlock()

	zap.L().Info("RabbitMQ connected", zap.Int("channels", size))
	return nil
}

// reconnectLoop 断线重连循环
func (p *ConnPool) reconnectLoop() {
	defer close(p.done)

	interval := time.Duration(p.cfg.Reconnect.Interval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.closed:
			return
		case <-ticker.C:
			p.mu.Lock()
			alive := p.conn != nil && !p.conn.IsClosed()
			p.mu.Unlock()
			if alive {
				continue
			}

			// 检查最大重试次数
			if p.cfg.Reconnect.MaxRetry > 0 && p.failCnt >= p.cfg.Reconnect.MaxRetry {
				zap.L().Error("RabbitMQ reconnect max retry reached",
					zap.Int("maxRetry", p.cfg.Reconnect.MaxRetry))
				return
			}

			zap.L().Warn("RabbitMQ disconnected, reconnecting...",
				zap.Int("failCnt", p.failCnt+1))
			if err := p.connect(); err != nil {
				p.failCnt++
				zap.L().Error("RabbitMQ reconnect failed", zap.Error(err))
			}
		}
	}
}

// GetChannel 获取一个可用 Channel（轮询方式）
func (p *ConnPool) GetChannel() (*amqp.Channel, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.channels) == 0 {
		return nil, fmt.Errorf("no available channels")
	}

	ch := p.channels[0]
	p.channels = append(p.channels[1:], ch)
	return ch, nil
}

// Publish 生产者专用投递方法，失败自动重试一次
func (p *ConnPool) Publish(body []byte) error {
	ch, err := p.GetChannel()
	if err != nil {
		return fmt.Errorf("get channel failed: %w", err)
	}

	err = ch.Publish(p.cfg.Click.Exchange, p.cfg.Click.RoutingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
		Timestamp:    time.Now(),
	})
	if err != nil {
		zap.L().Warn("publish failed, retrying...", zap.Error(err))
		ch2, err2 := p.GetChannel()
		if err2 != nil {
			return fmt.Errorf("get channel for retry failed: %w", err2)
		}
		return ch2.Publish(p.cfg.Click.Exchange, p.cfg.Click.RoutingKey, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
			Timestamp:    time.Now(),
		})
	}
	return nil
}

// Consume 返回一个消费者的消息通道
func (p *ConnPool) Consume() (<-chan amqp.Delivery, error) {
	ch, err := p.GetChannel()
	if err != nil {
		return nil, fmt.Errorf("get channel for consume failed: %w", err)
	}

	// 使用配置的预取数量
	if err := ch.Qos(p.cfg.Consumer.PrefetchCount, 0, false); err != nil {
		return nil, fmt.Errorf("set qos failed: %w", err)
	}

	// auto_ack 由调用方控制，这里始终返回手动 ACK 的 channel
	return ch.Consume(p.cfg.Click.Queue, "", p.cfg.Consumer.AutoAck, false, false, false, nil)
}

// Close 优雅关闭
func (p *ConnPool) Close() {
	close(p.closed)
	<-p.done

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, ch := range p.channels {
		ch.Close()
	}
	if p.conn != nil {
		p.conn.Close()
	}
	zap.L().Info("RabbitMQ connection pool closed")
}

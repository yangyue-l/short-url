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
	cfg       *settings.RabbitMQConfig
	mu        sync.Mutex
	conn      *amqp.Connection
	channels  []*amqp.Channel // 消费者 channel（轮询分配）
	prodCh    *amqp.Channel   // 生产者专用 channel
	confirmCh chan amqp.Confirmation
	pubMu     sync.Mutex // ★ 保护 Publish 整个操作（AMQP Channel 不能并发写）
	closed    chan struct{}
	done      chan struct{}
	failCnt   int // 连续重连失败计数
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

// connect 建立连接 + 申明交换机/队列(含 DLX) + 创建 Channel 池
func (p *ConnPool) connect() error {
	conn, err := amqp.Dial(p.cfg.Addr())
	if err != nil {
		return fmt.Errorf("dial RabbitMQ failed: %w", err)
	}

	// 初始化 —— 先声明拓扑（用临时 channel）
	setupCh, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("create setup channel failed: %w", err)
	}
	dlxName := p.cfg.Click.Exchange + ".dlx"
	dlqName := p.cfg.Click.Queue + ".dlq"
	// 死信交换机
	setupCh.ExchangeDeclare(dlxName, "direct", true, false, false, false, nil)
	// 死信队列
	setupCh.QueueDeclare(dlqName, true, false, false, false,
		amqp.Table{"x-message-ttl": int32(7 * 24 * 3600 * 1000)})
	setupCh.QueueBind(dlqName, "#", dlxName, false, nil)
	// 业务交换机
	setupCh.ExchangeDeclare(p.cfg.Click.Exchange, p.cfg.Click.ExchangeType,
		p.cfg.Click.Durable, p.cfg.Click.AutoDelete, false, false, nil)
	// 业务队列（绑定 DLX）
	setupCh.QueueDeclare(p.cfg.Click.Queue, p.cfg.Click.Durable, p.cfg.Click.AutoDelete,
		false, false, amqp.Table{
			"x-dead-letter-exchange":    dlxName,
			"x-dead-letter-routing-key": "#",
			"x-message-ttl":             int32(24 * 3600 * 1000),
		})
	setupCh.QueueBind(p.cfg.Click.Queue, p.cfg.Click.RoutingKey, p.cfg.Click.Exchange, false, nil)
	setupCh.Close()

	// ── 生产者专用 Channel（独立，Confirm 只注册一次）──
	prodCh, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("create producer channel failed: %w", err)
	}
	if err := prodCh.Confirm(false); err != nil {
		conn.Close()
		return fmt.Errorf("enable producer confirm failed: %w", err)
	}
	// ★ 只注册一次 NotifyPublish，存到 pool.confirmCh 供所有 Publish 调用复用
	confirmCh := prodCh.NotifyPublish(make(chan amqp.Confirmation, 10))

	// ── 消费者 Channel ──
	channels := make([]*amqp.Channel, 0, p.cfg.Consumer.Workers)
	for i := 0; i < p.cfg.Consumer.Workers; i++ {
		ch, err := conn.Channel()
		if err != nil {
			prodCh.Close()
			conn.Close()
			return fmt.Errorf("create consumer channel #%d failed: %w", i, err)
		}
		channels = append(channels, ch)
	}

	p.mu.Lock()
	p.conn = conn
	p.prodCh = prodCh
	p.confirmCh = confirmCh
	p.channels = channels
	p.failCnt = 0
	p.mu.Unlock()

	zap.L().Info("RabbitMQ connected",
		zap.Int("consumers", len(channels)),
		zap.Int("producers", 1))
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

// Publish 生产者专用投递（pubMu 串行化，AMQP Channel 不能并发写）
func (p *ConnPool) Publish(body []byte) error {
	p.pubMu.Lock()
	defer p.pubMu.Unlock()

	p.mu.Lock()
	ch := p.prodCh
	confirmCh := p.confirmCh
	p.mu.Unlock()

	if ch == nil {
		return fmt.Errorf("producer channel not available")
	}

	seq := ch.GetNextPublishSeqNo()
	err := ch.Publish(p.cfg.Click.Exchange, p.cfg.Click.RoutingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
		Timestamp:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}

	// 等待 DeliveryTag >= seq 的确认（共享 confirmCh，按 seq 顺序投递）
	for {
		select {
		case confirm, ok := <-confirmCh:
			if !ok {
				return fmt.Errorf("confirm channel closed")
			}
			if confirm.DeliveryTag >= seq {
				if !confirm.Ack {
					return fmt.Errorf("publish nack")
				}
				return nil
			}
			// DeliveryTag < seq 的是其他 goroutine 的，跳过继续等
		case <-time.After(5 * time.Second):
			return fmt.Errorf("publish confirm timeout")
		}
	}
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

	if p.prodCh != nil {
		p.prodCh.Close()
	}
	for _, ch := range p.channels {
		ch.Close()
	}
	if p.conn != nil {
		p.conn.Close()
	}
	zap.L().Info("RabbitMQ connection pool closed")
}

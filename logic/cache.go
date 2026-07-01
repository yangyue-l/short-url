package logic

import (
	"math/rand/v2"
	"short-url/dao/redis"
	"short-url/pkg/bloom"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

var (
	// BloomFilter 全局布隆过滤器（main.go 中初始化）
	BloomFilter *bloom.Filter
	// sfGroup 用于合并并发的缓存 miss 请求
	sfGroup singleflight.Group
)

// ─── 缓存写入 worker pool ───

const (
	cacheWorkerNum = 10
	cacheQueueSize = 10000
)

type cacheTask struct {
	shortCode string
	longURL   string
	ttl       time.Duration
}

var cacheCh = make(chan cacheTask, cacheQueueSize)

func init() {
	for i := 0; i < cacheWorkerNum; i++ {
		go cacheWorker()
	}
}

func cacheWorker() {
	for task := range cacheCh {
		if err := redis.CacheShortURL(task.shortCode, task.longURL, task.ttl); err != nil {
			zap.L().Warn("cache to redis failed",
				zap.String("shortCode", task.shortCode), zap.Error(err))
		}
	}
}

// jitterTTL 给 TTL 添加 ±25% 随机偏移，防止缓存雪崩
func jitterTTL(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}
	// 随机偏移范围：base * [0.75, 1.25]
	jitter := time.Duration(float64(base) * (0.75 + rand.Float64()*0.5))
	return jitter
}

// cacheURL 异步写 Redis 缓存（通过 worker pool，不阻塞主流程，自动加 TTL 抖动）
func cacheURL(shortCode, longURL string, ttl time.Duration) {
	select {
	case cacheCh <- cacheTask{shortCode, longURL, jitterTTL(ttl)}:
	default:
		// 队列满则丢弃，不影响主流程，下次请求会回源 MySQL
		zap.L().Warn("cache worker queue full, dropping cache task",
			zap.String("shortCode", shortCode))
	}
}

// ─── 布隆过滤器操作 ───

var bloomOnce sync.Once

// InitBloomFilter 初始化布隆过滤器（启动时调用一次）
func InitBloomFilter(n uint32, p float64) {
	bloomOnce.Do(func() {
		BloomFilter = bloom.New(n, p)
	})
}

// LoadBloomFromDB 从 MySQL 加载已有短码到布隆过滤器
func LoadBloomFromDB(codes []string) {
	if BloomFilter == nil {
		return
	}
	for _, code := range codes {
		BloomFilter.Add(code)
	}
	zap.L().Info("bloom filter loaded", zap.Int("codes", len(codes)))
}

// AddToBloom 新增短码时同步加入布隆过滤器
func AddToBloom(shortCode string) {
	if BloomFilter != nil {
		BloomFilter.Add(shortCode)
	}
}

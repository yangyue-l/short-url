package redis

import (
	"fmt"
	"time"

	"short-url/models"

	"github.com/go-redis/redis/v8"
)

const (
	ClickPVPrefix  = "click:pv:"
	ClickUVPrefix  = "click:uv:"
	ClickActiveSet = "click:active"
)

// BatchIncrPV 批量增加 PV 计数（Pipeline）
func BatchIncrPV(client *redis.Client, pvMap map[string]int64) {
	pipe := client.Pipeline()
	for code, delta := range pvMap {
		pipe.IncrBy(client.Context(), ClickPVPrefix+code, delta)
	}
	_, _ = pipe.Exec(client.Context())
}

// BatchAddUV 批量添加 UV（HyperLogLog）
func BatchAddUV(client *redis.Client, items []*models.ClickItem) {
	pipe := client.Pipeline()
	today := time.Now().Format("2006-01-02")
	for _, item := range items {
		key := fmt.Sprintf("%s%s:%s", ClickUVPrefix, item.ShortCode, today)
		pipe.PFAdd(client.Context(), key, item.IP)
	}
	_, _ = pipe.Exec(client.Context())
}

// MarkActive 标记活跃短链
func MarkActive(client *redis.Client, pvMap map[string]int64) {
	pipe := client.Pipeline()
	now := float64(time.Now().Unix())
	for code := range pvMap {
		pipe.ZAdd(client.Context(), ClickActiveSet, &redis.Z{Score: now, Member: code})
	}
	_, _ = pipe.Exec(client.Context())
}

// GetPV 获取实时 PV（Redis 增量部分）
func GetPV(client *redis.Client, shortCode string) int64 {
	val, err := client.Get(client.Context(), ClickPVPrefix+shortCode).Int64()
	if err != nil {
		return 0
	}
	return val
}

// GetUV 获取今日 UV 近似值（HyperLogLog）
func GetUV(client *redis.Client, shortCode string) int64 {
	today := time.Now().Format("2006-01-02")
	key := fmt.Sprintf("%s%s:%s", ClickUVPrefix, shortCode, today)
	val, err := client.PFCount(client.Context(), key).Result()
	if err != nil {
		return 0
	}
	return val
}

// GetActiveShortCodes 获取最近活跃的短链列表
func GetActiveShortCodes(client *redis.Client, since int64) ([]string, error) {
	return client.ZRangeByScore(client.Context(), ClickActiveSet, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", since),
		Max: "+inf",
	}).Result()
}

// FlushPV 回刷时将 Redis 中的增量 PV 取出并扣减
func FlushPV(client *redis.Client, shortCode string) int64 {
	key := ClickPVPrefix + shortCode
	val, err := client.GetSet(client.Context(), key, 0).Int64()
	if err != nil {
		return 0
	}
	return val
}

// CleanActive 回刷后从活跃集移除
func CleanActive(client *redis.Client, codes ...string) {
	client.ZRem(client.Context(), ClickActiveSet, codes)
}

package redis

import (
	"fmt"
	"sync/atomic"
	"time"

	"short-url/models"

	"github.com/go-redis/redis/v8"
)

const (
	ClickPVPrefix  = "click:pv:"
	ClickUVPrefix  = "click:uv:"
	ClickActiveSet = "click:active"

	// 数据 TTL：防止无 TTL key 导致 Redis 内存泄漏
	pvKeyTTL     = 48 * time.Hour // click:pv:* —— 48 小时未更新则自动清理
	uvKeyTTL     = 48 * time.Hour // click:uv:* —— 同上
	todayKeyTTL  = 48 * time.Hour // click:today:* —— 同上
	activeMaxAge = 2 * time.Hour  // click:active —— 2 小时前的记录可清理
)

var markActiveCounter int64 // 用于周期性清理 click:active ZSet

// BatchIncrPV 批量增加 PV 计数（Pipeline，带 TTL）
func BatchIncrPV(client *redis.Client, pvMap map[string]int64) {
	pipe := client.Pipeline()
	for code, delta := range pvMap {
		key := ClickPVPrefix + code
		pipe.IncrBy(client.Context(), key, delta)
		pipe.Expire(client.Context(), key, pvKeyTTL)
	}
	_, _ = pipe.Exec(client.Context())
}

// BatchAddUV 批量添加 UV（HyperLogLog，带 TTL）
func BatchAddUV(client *redis.Client, items []*models.ClickItem) {
	pipe := client.Pipeline()
	today := time.Now().Format("2006-01-02")
	for _, item := range items {
		key := fmt.Sprintf("%s%s:%s", ClickUVPrefix, item.ShortCode, today)
		pipe.PFAdd(client.Context(), key, item.IP)
		pipe.Expire(client.Context(), key, uvKeyTTL)
	}
	_, _ = pipe.Exec(client.Context())
}

// MarkActive 标记活跃短链（ZSet + 周期性清理旧记录）
func MarkActive(client *redis.Client, pvMap map[string]int64) {
	ctx := client.Context()
	pipe := client.Pipeline()
	now := float64(time.Now().Unix())
	for code := range pvMap {
		pipe.ZAdd(ctx, ClickActiveSet, &redis.Z{Score: now, Member: code})
	}

	// 每 100 次调用清理一次 2 小时前的旧记录，防止 ZSet 无限增长
	count := atomic.AddInt64(&markActiveCounter, 1)
	if count%100 == 0 {
		cutoff := time.Now().Add(-activeMaxAge).Unix()
		pipe.ZRemRangeByScore(ctx, ClickActiveSet, "0", fmt.Sprintf("%d", cutoff))
	}
	_, _ = pipe.Exec(ctx)
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

// ResetPV 回刷成功后将 Redis 中的 PV 计数重置为 0（带 TTL，防止 key 残留）
func ResetPV(client *redis.Client, shortCode string) {
	client.Set(client.Context(), ClickPVPrefix+shortCode, 0, pvKeyTTL)
}

// CleanActive 回刷后从活跃集移除
func CleanActive(client *redis.Client, codes ...string) {
	client.ZRem(client.Context(), ClickActiveSet, codes)
}

const ClickTodayPrefix = "click:today:"

// IncrTodayClick 增加今日全局点击数（带 TTL，48h 后自动过期）
func IncrTodayClick(client *redis.Client, delta int64) {
	ctx := client.Context()
	today := time.Now().Format("2006-01-02")
	key := ClickTodayPrefix + today
	pipe := client.Pipeline()
	pipe.IncrBy(ctx, key, delta)
	pipe.Expire(ctx, key, todayKeyTTL)
	_, _ = pipe.Exec(ctx)
}

// GetTodayClick 获取今日全局点击数（Redis 侧）
func GetTodayClick(client *redis.Client) int64 {
	today := time.Now().Format("2006-01-02")
	val, err := client.Get(client.Context(), ClickTodayPrefix+today).Int64()
	if err != nil {
		return 0
	}
	return val
}

// SumActivePV 汇总所有活跃短码的 Redis PV 增量
func SumActivePV(client *redis.Client, since int64) int64 {
	codes, err := GetActiveShortCodes(client, since)
	if err != nil || len(codes) == 0 {
		return 0
	}
	var total int64
	for _, code := range codes {
		val, err := client.Get(client.Context(), ClickPVPrefix+code).Int64()
		if err == nil {
			total += val
		}
	}
	return total
}

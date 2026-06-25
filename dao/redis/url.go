package redis

import (
	"time"
)

const (
	URLCachePrefix  = "short:url:"
	URLCacheTTL     = 24 * time.Hour
	RequestIDPrefix = "short:request:"
	RequestIDTTL    = 1 * time.Hour
	ShortCodeSeqKey = "short:code:seq"
	ShortCodeSeqTTL = 0 // 永不过期，保证序列不丢失
)

func CacheShortURL(shortCode, longURL string, ttl time.Duration) error {
	return rdb.Set(ctx, URLCachePrefix+shortCode, longURL, ttl).Err()
}

func GetCachedURL(shortCode string) (string, error) {
	return rdb.Get(ctx, URLCachePrefix+shortCode).Result()
}

func DeleteCache(shortCode string) error {
	return rdb.Del(ctx, URLCachePrefix+shortCode).Err()
}

func SetRequestID(requestID string) error {
	return rdb.Set(ctx, RequestIDPrefix+requestID, requestID, RequestIDTTL).Err()
}

func GetRequestID(requestID string) (string, error) {
	return rdb.Get(ctx, RequestIDPrefix+requestID).Result()
}

// GetNextShortCodeSeq 获取下一个短码序列号（原子自增），用于预生成短码
func GetNextShortCodeSeq() (uint64, error) {
	return rdb.Incr(ctx, ShortCodeSeqKey).Uint64()
}

func InitShortCodeSeq(startID uint64) error {
	return rdb.SetNX(ctx, ShortCodeSeqKey, startID, ShortCodeSeqTTL).Err()
}

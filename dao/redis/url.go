package redis

import "time"

const (
	URLCachePrefix = "short:url:"
	URLCacheTTL    = 24 * time.Hour
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

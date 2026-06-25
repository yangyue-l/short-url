package logic

import (
	"errors"
	"fmt"
	"time"
	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/models"
	"short-url/pkg/base62"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// CreateShortURL 创建短链接
func CreateShortURL(longURL string, expireIn int64) (*models.ShortenResponse, error) {
	url := &models.URL{
		LongURL: longURL,
	}
	if expireIn > 0 {
		t := time.Now().Add(time.Duration(expireIn) * time.Second)
		url.ExpireAt = &t
	}

	// 先创建记录，获取自增 ID
	if err := mysql.CreateURL(url); err != nil {
		zap.L().Error("create url failed", zap.Error(err))
		return nil, errors.New("create url failed")
	}

	// 用 ID 生成短码
	shortCode := base62.Encode(url.ID)
	url.ShortCode = shortCode
	if err := mysql.UpdateShortCode(url.ID, shortCode); err != nil {
		zap.L().Error("update short code failed", zap.Error(err))
		return nil, errors.New("update short code failed")
	}

	// 缓存到 Redis
	ttl := redis.URLCacheTTL
	if expireIn > 0 && time.Duration(expireIn)*time.Second < ttl {
		ttl = time.Duration(expireIn) * time.Second
	}
	if err := redis.CacheShortURL(shortCode, longURL, ttl); err != nil {
		zap.L().Warn("cache to redis failed", zap.Error(err))
	}

	resp := &models.ShortenResponse{
		ShortURL:  fmt.Sprintf("http://localhost:%d/%s", 8080, shortCode),
		ShortCode: shortCode,
		LongURL:   longURL,
	}
	if url.ExpireAt != nil {
		resp.ExpireAt = url.ExpireAt.Format(time.RFC3339)
	}

	return resp, nil
}

// GetLongURL 通过短码获取原始链接
func GetLongURL(shortCode string) (string, error) {
	// 先从 Redis 缓存查询
	longURL, err := redis.GetCachedURL(shortCode)
	if err == nil {
		go func() {
			if e := mysql.IncrementClickCnt(shortCode); e != nil {
				zap.L().Warn("increment click count failed", zap.Error(e))
			}
		}()
		return longURL, nil
	}

	// 缓存未命中，从 MySQL 查询
	url, err := mysql.GetURLByShortCode(shortCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("short code not found")
		}
		zap.L().Error("get url by short code failed", zap.Error(err))
		return "", errors.New("query failed")
	}

	// 检查是否过期
	if url.ExpireAt != nil && url.ExpireAt.Before(time.Now()) {
		return "", errors.New("short code has expired")
	}

	// 回写 Redis 缓存
	ttl := redis.URLCacheTTL
	if url.ExpireAt != nil {
		remaining := time.Until(*url.ExpireAt)
		if remaining < ttl {
			ttl = remaining
		}
	}
	if err := redis.CacheShortURL(shortCode, url.LongURL, ttl); err != nil {
		zap.L().Warn("cache to redis failed", zap.Error(err))
	}

	// 异步增加点击计数
	go func() {
		if e := mysql.IncrementClickCnt(shortCode); e != nil {
			zap.L().Warn("increment click count failed", zap.Error(e))
		}
	}()

	return url.LongURL, nil
}

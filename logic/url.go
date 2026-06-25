package logic

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/models"
	"short-url/pkg/base62"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrRequestAlreadyProcessed = errors.New("request already processed")
)

// generateShortCode 预生成一个随机短码（64-bit 随机数 Base62 编码）
// 碰撞概率极低（2^64 空间），一次 INSERT 即可完成创建
func generateShortCode() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极端情况下 crypto/rand 失败，回退到纳秒时间戳
		binary.BigEndian.PutUint64(b[:], uint64(time.Now().UnixNano()))
	}
	return base62.Encode(binary.BigEndian.Uint64(b[:]))
}

// CreateShortURL 创建短链接
func CreateShortURL(longURL string, expireIn int64) (*models.ParamShortenResponse, error) {
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

	resp := &models.ParamShortenResponse{
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
	if url == nil {
		zap.L().Warn("mysql.GetURLByShortCode returned nil without error", zap.String("shortCode", shortCode))
		return "", errors.New("query returned nil")
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

// GetShortenInfo 查询短链接信息
func GetShortenInfo(shortCode string) (*models.ParamURLInfoResponse, error) {
	url, err := mysql.GetURLByShortCode(shortCode)
	if err != nil {
		zap.L().Warn("mysql.GetURLByShortCode(shortCode) failed", zap.Error(err))
		return nil, err
	}
	if url == nil {
		zap.L().Warn("mysql.GetURLByShortCode returned nil without error", zap.String("shortCode", shortCode))
		return nil, errors.New("query returned nil")
	}

	// 判断是否过期（ExpireAt 为 nil 表示永不过期）
	isExpired := false
	if url.ExpireAt != nil && url.ExpireAt.Before(time.Now()) {
		isExpired = true
	}

	urlInfo := &models.ParamURLInfoResponse{
		ShortCode: url.ShortCode,
		LongURL:   url.LongURL,
		ClickCnt:  url.ClickCnt,
		IsExpired: isExpired,
		CreatedAt: url.CreatedAt.Format(time.RFC3339),
		UpdatedAt: url.UpdatedAt.Format(time.RFC3339),
	}
	if url.ExpireAt != nil {
		urlInfo.ExpireAt = url.ExpireAt.Format(time.RFC3339)
	}
	return urlInfo, nil
}

// CreateBatchShortURL 批量创建短链接
func CreateBatchShortURL(p *models.ParamBatchURLRequest) (*models.ParamBatchURLResponse, error) {
	// 幂等性校验：如果提供了 request_id，检查是否已处理过
	if p.RequestID != "" {
		existing, _ := redis.GetRequestID(p.RequestID)
		if existing != "" {
			return nil, ErrRequestAlreadyProcessed
		}
		if err := redis.SetRequestID(p.RequestID); err != nil {
			zap.L().Error("set request id failed", zap.Error(err))
			return nil, errors.New("request idempotency check failed")
		}
	}

	resp := &models.ParamBatchURLResponse{
		Results: make([]models.ParamURLsResponse, 0, len(p.URLs)),
		Total:   len(p.URLs),
	}

	for _, item := range p.URLs {
		itemResp := models.ParamURLsResponse{
			LongURL: item.LongURL,
			Success: false,
		}

		url := &models.URL{
			LongURL: item.LongURL,
		}
		if item.ExpireIn > 0 {
			expireAt := time.Now().Add(time.Duration(item.ExpireIn) * time.Second)
			url.ExpireAt = &expireAt
		}

		// 预生成短码：自定义码直接用，自动码用 64-bit 随机数编码
		// 一条 INSERT 完成创建，无需二次 UPDATE
		if item.CustomCode != "" {
			url.ShortCode = item.CustomCode
		} else {
			url.ShortCode = generateShortCode()
		}

		if err := mysql.CreateURL(url); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				if item.CustomCode != "" {
					itemResp.Error = "custom code already exists"
				} else {
					itemResp.Error = "short code collision, please retry"
				}
			} else {
				zap.L().Error("batch create url failed", zap.Error(err))
				itemResp.Error = "internal error"
			}
			resp.FailCount++
			resp.Results = append(resp.Results, itemResp)
			continue
		}

		// 异步缓存到 Redis，不阻塞响应
		// TTL 策略：取过期时间和默认缓存时间（24h）的较小值
		// - 链接过期时间 < 24h -> 缓存随链接一起过期
		// - 永不过期或过期时间 > 24h -> 使用默认缓存时间
		ttl := redis.URLCacheTTL
		if item.ExpireIn > 0 && time.Duration(item.ExpireIn)*time.Second < ttl {
			ttl = time.Duration(item.ExpireIn) * time.Second
		}
		go func(sc, lu string, t time.Duration) {
			if err := redis.CacheShortURL(sc, lu, t); err != nil {
				zap.L().Warn("batch cache to redis failed",
					zap.String("shortCode", sc), zap.Error(err))
			}
		}(url.ShortCode, item.LongURL, ttl)

		itemResp.ShortURL = fmt.Sprintf("http://localhost:%d/%s", 8080, url.ShortCode)
		itemResp.ShortCode = url.ShortCode
		itemResp.Success = true
		resp.SuccessCount++
		resp.Results = append(resp.Results, itemResp)
	}

	return resp, nil
}

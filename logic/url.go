package logic

import (
	"errors"
	"fmt"
	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/models"
	"short-url/pkg/base62"
	"short-url/settings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrRequestAlreadyProcessed = errors.New("request already processed")
)

// createURL 创建一条 URL 记录。自定义短码直接插入，自动短码先插入再回写编码后的自增 ID
func createURL(url *models.URL) error {
	if err := mysql.CreateURL(url); err != nil {
		return err
	}
	// 自动短码：用 MySQL 自增 ID 做 Base62 编码，保证唯一且不依赖任何外部组件
	if url.ShortCode == "" {
		shortCode := base62.Encode(url.ID)
		if err := mysql.UpdateShortCode(url.ID, shortCode); err != nil {
			return err
		}
		url.ShortCode = shortCode
	}
	return nil
}

// cacheURL 异步将短链接写入 Redis 缓存（丢失败不阻塞主流程）
func cacheURL(shortCode, longURL string, ttl time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error("cacheURL panic", zap.Any("recover", r))
			}
		}()
		if err := redis.CacheShortURL(shortCode, longURL, ttl); err != nil {
			zap.L().Warn("cache to redis failed",
				zap.String("shortCode", shortCode), zap.Error(err))
		}
	}()
}

// CreateShortURL 创建短链接
func CreateShortURL(userID uint64, longURL, customCode string, expireIn int64) (*models.ParamShortenResponse, error) {
	url := &models.URL{
		LongURL:   longURL,
		ShortCode: customCode,
		UserID:    &userID,
	}
	if expireIn > 0 {
		t := time.Now().Add(time.Duration(expireIn) * time.Second)
		url.ExpireAt = &t
	}

	if err := createURL(url); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, errors.New("custom code already exists")
		}
		zap.L().Error("create url failed", zap.Error(err))
		return nil, errors.New("create url failed")
	}

	// 异步缓存
	ttl := redis.URLCacheTTL
	if expireIn > 0 && time.Duration(expireIn)*time.Second < ttl {
		ttl = time.Duration(expireIn) * time.Second
	}
	cacheURL(url.ShortCode, longURL, ttl)

	baseURL := settings.Cfg.BaseURL()
	resp := &models.ParamShortenResponse{
		ShortURL:  fmt.Sprintf("%s/%s", baseURL, url.ShortCode),
		ShortCode: url.ShortCode,
		LongURL:   longURL,
		CreatedAt: url.CreatedAt.Format(time.RFC3339),
	}
	if url.ExpireAt != nil {
		resp.ExpireAt = url.ExpireAt.Format(time.RFC3339)
	}

	return resp, nil
}

// GetLongURL 通过短码获取原始链接
func GetLongURL(shortCode string) (string, error) {
	// 先查 Redis 缓存
	longURL, err := redis.GetCachedURL(shortCode)
	if err == nil {
		recordClick(shortCode)
		return longURL, nil
	}

	// 缓存未命中，查 MySQL
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
		if remaining := time.Until(*url.ExpireAt); remaining < ttl {
			ttl = remaining
		}
	}
	cacheURL(shortCode, url.LongURL, ttl)

	// 异步记录点击
	recordClick(shortCode)

	return url.LongURL, nil
}

// GetShortenInfo 查询短链接信息
func GetShortenInfo(shortCode string) (*models.ParamURLInfoResponse, error) {
	url, err := mysql.GetURLByShortCode(shortCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("short code not found")
		}
		zap.L().Error("get url by short code failed", zap.Error(err))
		return nil, errors.New("query failed")
	}

	isExpired := false
	if url.ExpireAt != nil && url.ExpireAt.Before(time.Now()) {
		isExpired = true
	}

	info := &models.ParamURLInfoResponse{
		ShortCode: url.ShortCode,
		LongURL:   url.LongURL,
		ClickCnt:  url.ClickCnt,
		IsExpired: isExpired,
		CreatedAt: url.CreatedAt.Format(time.RFC3339),
		UpdatedAt: url.UpdatedAt.Format(time.RFC3339),
	}
	if url.ExpireAt != nil {
		info.ExpireAt = url.ExpireAt.Format(time.RFC3339)
	}
	return info, nil
}

// CreateBatchShortURL 批量创建短链接
func CreateBatchShortURL(userID uint64, p *models.ParamBatchURLRequest) (*models.ParamBatchURLResponse, error) {
	// 幂等性校验：使用 Redis SETNX 原子操作，避免 TOCTOU 竞态
	if p.RequestID != "" {
		ok, err := redis.SetRequestIDNX(p.RequestID)
		if err != nil {
			zap.L().Error("set request id failed", zap.Error(err))
			return nil, errors.New("request idempotency check failed")
		}
		if !ok {
			return nil, ErrRequestAlreadyProcessed
		}
	}

	resp := &models.ParamBatchURLResponse{
		Results: make([]models.ParamURLsResponse, 0, len(p.URLs)),
		Total:   len(p.URLs),
	}
	baseURL := settings.Cfg.BaseURL()

	for _, item := range p.URLs {
		itemResp := models.ParamURLsResponse{
			LongURL: item.LongURL,
			Success: false,
		}

		url := &models.URL{
			LongURL: item.LongURL,
			UserID:  &userID,
		}
		if item.CustomCode != "" {
			url.ShortCode = item.CustomCode
		}
		if item.ExpireIn > 0 {
			expireAt := time.Now().Add(time.Duration(item.ExpireIn) * time.Second)
			url.ExpireAt = &expireAt
		}

		if err := createURL(url); err != nil {
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

		// 异步缓存
		ttl := redis.URLCacheTTL
		if item.ExpireIn > 0 && time.Duration(item.ExpireIn)*time.Second < ttl {
			ttl = time.Duration(item.ExpireIn) * time.Second
		}
		cacheURL(url.ShortCode, item.LongURL, ttl)

		itemResp.ShortURL = fmt.Sprintf("%s/%s", baseURL, url.ShortCode)
		itemResp.ShortCode = url.ShortCode
		itemResp.Success = true
		resp.SuccessCount++
		resp.Results = append(resp.Results, itemResp)
	}

	return resp, nil
}

// UpdateShortCode 更新短链接（需要 ownership 校验）
func UpdateShortCode(userID uint64, shortCode, longURL string, expireIn int64) (*models.ParamUpdateResponse, error) {
	// TODO: 校验 userID 是否为该 shortCode 的创建者，更新 longURL
	return nil, nil
}

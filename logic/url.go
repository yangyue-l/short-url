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

func createURL(url *models.URL) error {
	if err := mysql.CreateURL(url); err != nil {
		return err
	}
	if url.ShortCode == "" {
		shortCode := base62.Encode(url.ID)
		if err := mysql.UpdateShortCode(url.ID, shortCode); err != nil {
			return err
		}
		url.ShortCode = shortCode
	}
	return nil
}

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

func CreateShortURL(userID int64, longURL, customCode string, expireIn int64) (*models.ParamShortenResponse, error) {
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

func GetLongURL(shortCode string) (string, error) {
	longURL, err := redis.GetCachedURL(shortCode)
	if err == nil {
		return longURL, nil
	}

	url, err := mysql.GetURLByShortCode(shortCode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errors.New("short code not found")
		}
		zap.L().Error("get url by short code failed", zap.Error(err))
		return "", errors.New("query failed")
	}

	if url.ExpireAt != nil && url.ExpireAt.Before(time.Now()) {
		return "", errors.New("short code has expired")
	}

	ttl := redis.URLCacheTTL
	if url.ExpireAt != nil {
		if remaining := time.Until(*url.ExpireAt); remaining < ttl {
			ttl = remaining
		}
	}
	cacheURL(shortCode, url.LongURL, ttl)

	return url.LongURL, nil
}

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

	clickCnt := url.ClickCnt + redis.GetPV(redis.GetClient(), shortCode)

	info := &models.ParamURLInfoResponse{
		ShortCode: url.ShortCode,
		LongURL:   url.LongURL,
		ClickCnt:  clickCnt,
		IsExpired: isExpired,
		CreatedAt: url.CreatedAt.Format(time.RFC3339),
		UpdatedAt: url.UpdatedAt.Format(time.RFC3339),
	}
	if url.ExpireAt != nil {
		info.ExpireAt = url.ExpireAt.Format(time.RFC3339)
	}
	return info, nil
}

func CreateBatchShortURL(userID int64, p *models.ParamBatchURLRequest) (*models.ParamBatchURLResponse, error) {
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

func UpdateLongURL(userID int64, shortCode string, p *models.ParamUpdateRequest) (*models.ParamUpdateResponse, error) {
	url, err := mysql.GetURLByShortCodeAndUser(shortCode, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("短链接不存在或无权操作")
		}
		return nil, err
	}

	var expireAt *time.Time
	if p.ExpireIn > 0 {
		t := time.Now().Add(time.Duration(p.ExpireIn) * time.Second)
		expireAt = &t
	}

	if err := mysql.UpdateURL(shortCode, p.LongURL, expireAt); err != nil {
		zap.L().Error("update url failed", zap.Error(err))
		return nil, errors.New("update failed")
	}

	_ = redis.DeleteCache(shortCode)

	resp := &models.ParamUpdateResponse{
		ShortCode: shortCode,
		LongURL:   p.LongURL,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	if expireAt != nil {
		resp.ExpireAt = expireAt.Format(time.RFC3339)
	} else if url.ExpireAt != nil {
		resp.ExpireAt = url.ExpireAt.Format(time.RFC3339)
	}

	return resp, nil
}

func DeleteShortURL(userID int64, shortCode string) error {
	if err := mysql.DeleteURL(userID, shortCode); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("短链接不存在或无权操作")
		}
		return err
	}
	_ = redis.DeleteCache(shortCode)
	return nil
}

func GetUserURLs(userID int64, page, pageSize int) (*models.ParamUserURLsResponse, error) {
	urls, total, err := mysql.GetURLsByUserID(userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	baseURL := settings.Cfg.BaseURL()
	list := make([]*models.ParamUserURLsList, 0, len(urls))
	client := redis.GetClient()

	for _, url := range urls {
		clickCnt := url.ClickCnt + redis.GetPV(client, url.ShortCode)

		item := &models.ParamUserURLsList{
			ShortCode: url.ShortCode,
			ShortURL:  fmt.Sprintf("%s/%s", baseURL, url.ShortCode),
			LongURL:   url.LongURL,
			ClickCnt:  clickCnt,
			IsExpired: url.ExpireAt != nil && url.ExpireAt.Before(time.Now()),
			CreatedAt: url.CreatedAt.Format(time.RFC3339),
		}
		if url.ExpireAt != nil {
			item.ExpireAt = url.ExpireAt.Format(time.RFC3339)
		}

		list = append(list, item)
	}
	return &models.ParamUserURLsResponse{
		List:     list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetShortStats 获取短链接详情统计（合并 Redis 实时 + MySQL 历史数据）
func GetShortStats(userID int64, shortCode string) (*models.ParamShortStatsResponse, error) {
	url, err := mysql.GetURLByShortCodeAndUser(shortCode, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("短链接不存在或无权操作")
		}
		return nil, err
	}

	stats, err := mysql.GetClickStats(shortCode)
	if err != nil {
		return nil, err
	}

	client := redis.GetClient()

	// 合并 Redis 实时 PV
	redisPV := redis.GetPV(client, shortCode)
	totalClicks := url.ClickCnt + redisPV

	// 合并 Redis 实时 UV（HyperLogLog）
	redisUV := redis.GetUV(client, shortCode)
	uniqueIPs := stats.UniqueIPs + redisUV
	todayClicks := stats.TodayClicks + redisPV

	// 浏览器解析——从 DAO 拿原始数据，在 logic 层做归类
	rawBrowsers, _ := mysql.GetBrowserData(shortCode)
	topBrowsers := mergeBrowsers(rawBrowsers, 5)

	return &models.ParamShortStatsResponse{
		ShortCode:   shortCode,
		TotalClicks: totalClicks,
		UniqueIPs:   uniqueIPs,
		TodayClicks: todayClicks,
		ClicksByDay: stats.ClicksByDay,
		TopReferers: stats.TopReferers,
		TopBrowsers: topBrowsers,
	}, nil
}

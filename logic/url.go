package logic

import (
	"encoding/json"
	"errors"
	"fmt"
	"short-url/dao/mysql"
	"short-url/dao/redis"
	"short-url/models"
	"short-url/pkg/base62"
	"short-url/settings"
	"sync"
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
	AddToBloom(url.ShortCode) // 新增短码加入布隆过滤器

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
	// 第一层：布隆过滤器快速拦截不存在的短码
	if BloomFilter != nil && !BloomFilter.MayExist(shortCode) {
		return "", errors.New("short code not found")
	}

	// 第二层：singleflight 合并并发请求，防止缓存击穿
	val, err, _ := sfGroup.Do(shortCode, func() (interface{}, error) {
		longURL, err := redis.GetCachedURL(shortCode)
		if err == nil {
			return longURL, nil
		}

		url, err := mysql.GetURLByShortCode(shortCode)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("short code not found")
			}
			zap.L().Error("get url by short code failed", zap.Error(err))
			return nil, errors.New("query failed")
		}

		if url.ExpireAt != nil && url.ExpireAt.Before(time.Now()) {
			return nil, errors.New("short code has expired")
		}

		ttl := redis.URLCacheTTL
		if url.ExpireAt != nil {
			if remaining := time.Until(*url.ExpireAt); remaining < ttl {
				ttl = remaining
			}
		}
		cacheURL(shortCode, url.LongURL, ttl)

		return url.LongURL, nil
	})

	if err != nil {
		return "", err
	}
	return val.(string), nil
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
		AddToBloom(url.ShortCode)

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

// GetAdminURLs 管理员分页查询全部短链接（合并 Redis 实时 PV）
func GetAdminURLs(page, pageSize int, keyword, status, sort, order string) (*models.ParamAdminURLsResponse, error) {
	urls, total, err := mysql.GetAdminURLs(page, pageSize, keyword, status, sort, order)
	if err != nil {
		return nil, err
	}

	baseURL := settings.Cfg.BaseURL()
	client := redis.GetClient()
	list := make([]*models.ParamAdminURLsList, 0, len(urls))

	for _, u := range urls {
		clickCnt := u.ClickCnt + redis.GetPV(client, u.ShortCode)

		var userID int64
		if u.UserID != nil {
			userID = *u.UserID
		}

		item := &models.ParamAdminURLsList{
			ShortCode: u.ShortCode,
			ShortURL:  fmt.Sprintf("%s/%s", baseURL, u.ShortCode),
			LongURL:   u.LongURL,
			ClickCnt:  clickCnt,
			IsExpired: u.ExpireAt != nil && u.ExpireAt.Before(time.Now()),
			CreatedAt: u.CreatedAt.Format(time.RFC3339),
			UserID:    userID,
			Username:  u.Username,
		}
		list = append(list, item)
	}

	return &models.ParamAdminURLsResponse{
		List:     list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetStatsOverview 获取全局统计概览（并行查询 + Redis 缓存 1 分钟）
func GetStatsOverview() (*models.ParamStatsOverviewResponse, error) {
	client := redis.GetClient()

	// 检查缓存
	const cacheKey = "stats:overview"
	const cacheTTL = 1 * time.Minute
	cached, err := client.Get(client.Context(), cacheKey).Result()
	if err == nil && cached != "" {
		var resp models.ParamStatsOverviewResponse
		if err := json.Unmarshal([]byte(cached), &resp); err == nil {
			return &resp, nil
		}
	}

	// 并行查询 MySQL
	var (
		urlStats   *mysql.URLStats
		totalMySQL int64
		todayMySQL int64
		urlErr     error
		clickErr   error
		wg         sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		urlStats, urlErr = mysql.GetURLStats()
	}()
	go func() {
		defer wg.Done()
		totalMySQL, todayMySQL, clickErr = mysql.GetGlobalClickStats()
	}()
	wg.Wait()

	if urlErr != nil {
		return nil, urlErr
	}
	if clickErr != nil {
		return nil, clickErr
	}

	// 合并 Redis 未回刷数据
	redisPV := redis.SumActivePV(client, time.Now().Unix()-60*60)
	redisToday := redis.GetTodayClick(client)

	totalClicks := totalMySQL + redisPV
	todayClicks := todayMySQL + redisToday

	resp := &models.ParamStatsOverviewResponse{
		TotalURLs:    urlStats.TotalURLs,
		TotalClicks:  totalClicks,
		ActiveURLs:   urlStats.ActiveURLs,
		ExpiredURLs:  urlStats.ExpiredURLs,
		TodayCreated: urlStats.TodayCreated,
		TodayClicks:  todayClicks,
	}

	// 写入缓存
	go func() {
		data, _ := json.Marshal(resp)
		client.Set(client.Context(), cacheKey, data, cacheTTL)
	}()

	return resp, nil
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

	// 按天统计：优先使用预聚合表（更快），fallback 到 click_logs 原始扫描
	clicksByDay := stats.ClicksByDay
	if dailyRows, err := mysql.GetClickStatsDaily(shortCode); err == nil && len(dailyRows) > 0 {
		todayStr := time.Now().Format("2006-01-02")
		for i := range dailyRows {
			if dailyRows[i].Date == todayStr {
				dailyRows[i].Count += redisPV // 加上 Redis 中尚未预聚合的实时 PV
			}
		}
		clicksByDay = dailyRows
	}

	// 浏览器解析——从 DAO 拿原始数据，在 logic 层做归类
	rawBrowsers, _ := mysql.GetBrowserData(shortCode)
	topBrowsers := mergeBrowsers(rawBrowsers, 5)

	return &models.ParamShortStatsResponse{
		ShortCode:   shortCode,
		TotalClicks: totalClicks,
		UniqueIPs:   uniqueIPs,
		TodayClicks: todayClicks,
		ClicksByDay: clicksByDay,
		TopReferers: stats.TopReferers,
		TopBrowsers: topBrowsers,
	}, nil
}

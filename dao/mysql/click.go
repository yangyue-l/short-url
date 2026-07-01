package mysql

import (
	"short-url/models"
	"time"

	"gorm.io/gorm"
)

func CreateClickLog(log *models.ClickLog) error {
	return db.Create(log).Error
}

// BatchCreateClickLogs 批量创建点击日志
func BatchCreateClickLogs(items []*models.ClickItem) error {
	if len(items) == 0 {
		return nil
	}
	logs := make([]*models.ClickLog, 0, len(items))
	for _, item := range items {
		logs = append(logs, &models.ClickLog{
			ShortCode: item.ShortCode,
			IP:        item.IP,
			Referer:   item.Referer,
			UserAgent: item.UserAgent,
			CreatedAt: time.UnixMilli(item.Timestamp),
		})
	}
	return db.CreateInBatches(logs, 200).Error
}

// IncrementClickCntBatch 批量累加 url 表的 click_cnt（使用 CASE WHEN 单条 UPDATE）
func IncrementClickCntBatch(pvMap map[string]int64) error {
	if len(pvMap) == 0 {
		return nil
	}

	// 构建 CASE WHEN 批量更新：一条 SQL 完成所有更新
	// UPDATE urls SET click_cnt = CASE short_code
	//   WHEN 'abc' THEN click_cnt + 10
	//   WHEN 'xyz' THEN click_cnt + 5
	// END WHERE short_code IN ('abc', 'xyz')
	codes := make([]string, 0, len(pvMap))
	caseSQL := "CASE short_code "
	args := make([]interface{}, 0, len(pvMap)*2)
	for code, delta := range pvMap {
		codes = append(codes, code)
		caseSQL += "WHEN ? THEN click_cnt + ? "
		args = append(args, code, delta)
	}
	caseSQL += "END"

	return db.Model(&models.URL{}).
		Where("short_code IN ?", codes).
		UpdateColumn("click_cnt", gorm.Expr(caseSQL, args...)).Error
}

// GetClickStats 查询详细统计（总数 / 独立IP / 今日点击数 / 按天 / 来源 Top5）
func GetClickStats(shortCode string) (*models.ClickStats, error) {
	stats := &models.ClickStats{}
	today := time.Now().Truncate(24 * time.Hour)

	if err := db.Model(&models.ClickLog{}).
		Where("short_code = ?", shortCode).Count(&stats.TotalClicks).Error; err != nil {
		return nil, err
	}

	if err := db.Model(&models.ClickLog{}).
		Where("short_code = ?", shortCode).
		Distinct("ip").Count(&stats.UniqueIPs).Error; err != nil {
		return nil, err
	}

	if err := db.Model(&models.ClickLog{}).
		Where("short_code = ? AND created_at >= ?", shortCode, today).Count(&stats.TodayClicks).Error; err != nil {
		return nil, err
	}

	// 近 7 天按天统计
	sevenDaysAgo := today.AddDate(0, 0, -6)
	rows, err := db.Model(&models.ClickLog{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("short_code = ? AND created_at >= ?", shortCode, sevenDaysAgo).
		Group("DATE(created_at)").
		Order("date ASC").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item models.ClicksByDay
		if err := rows.Scan(&item.Date, &item.Count); err != nil {
			return nil, err
		}
		stats.ClicksByDay = append(stats.ClicksByDay, item)
	}

	// 来源 Top 5
	refRows, err := db.Model(&models.ClickLog{}).
		Select("referer, COUNT(*) as count").
		Where("short_code = ? AND referer != ''", shortCode).
		Group("referer").
		Order("count DESC").
		Limit(5).
		Rows()
	if err != nil {
		return nil, err
	}
	defer refRows.Close()
	for refRows.Next() {
		var item models.TopSource
		if err := refRows.Scan(&item.Source, &item.Count); err != nil {
			return nil, err
		}
		stats.TopReferers = append(stats.TopReferers, item)
	}

	return stats, nil
}

// GetGlobalClickStats 获取全局点击统计（MySQL 侧）
func GetGlobalClickStats() (totalMySQL int64, todayMySQL int64, err error) {
	if err := db.Model(&models.URL{}).Select("COALESCE(SUM(click_cnt), 0)").Scan(&totalMySQL).Error; err != nil {
		return 0, 0, err
	}

	today := time.Now().Truncate(24 * time.Hour)
	if err := db.Model(&models.ClickLog{}).Where("created_at >= ?", today).Count(&todayMySQL).Error; err != nil {
		return 0, 0, err
	}

	return totalMySQL, todayMySQL, nil
}

// GetBrowserData 返回原始 UserAgent → 计数的映射，供 logic 层做浏览器解析
func GetBrowserData(shortCode string) (map[string]int64, error) {
	rows, err := db.Model(&models.ClickLog{}).
		Select("user_agent, COUNT(*) as count").
		Where("short_code = ? AND user_agent != ''", shortCode).
		Group("user_agent").
		Order("count DESC").
		Limit(20).
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var ua string
		var count int64
		if err := rows.Scan(&ua, &count); err != nil {
			return nil, err
		}
		result[ua] = count
	}
	return result, nil
}

// CleanOldClickLogs 清理超过 retentionDays 天的点击日志
func CleanOldClickLogs(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result := db.Where("created_at < ?", cutoff).Delete(&models.ClickLog{})
	return result.RowsAffected, result.Error
}

// GetClickStatsDaily 从预聚合表查询按天统计（近 7 天）
func GetClickStatsDaily(shortCode string) ([]models.ClicksByDay, error) {
	sevenDaysAgo := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -6)
	var rows []models.ClicksByDay
	err := db.Model(&models.ClickStatsDaily{}).
		Select("stat_date as date, SUM(pv) as count").
		Where("short_code = ? AND stat_date >= ?", shortCode, sevenDaysAgo.Format("2006-01-02")).
		Group("stat_date").
		Order("stat_date ASC").
		Find(&rows).Error
	return rows, err
}

// UpsertClickStatsDaily 按天更新预聚合统计表（ON DUPLICATE KEY UPDATE 累加模式）
func UpsertClickStatsDaily(date string, shortCode string, pvDelta, uvDelta int64) error {
	// 使用原生 SQL 确保 UPSERT 时累加而非覆盖
	sql := `INSERT INTO click_stats_daily (short_code, stat_date, pv, uv, created_at, updated_at)
		VALUES (?, ?, ?, ?, NOW(), NOW())
		ON DUPLICATE KEY UPDATE
			pv = pv + VALUES(pv),
			uv = uv + VALUES(uv),
			updated_at = NOW()`
	return db.Exec(sql, shortCode, date, pvDelta, uvDelta).Error
}

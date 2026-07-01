package mysql

import (
	"short-url/models"
	"strings"
	"time"

	"gorm.io/gorm"
)

func CreateURL(url *models.URL) error {
	return db.Create(url).Error
}

func GetURLByShortCode(shortCode string) (*models.URL, error) {
	var url models.URL
	err := db.Where("short_code = ?", shortCode).First(&url).Error
	if err != nil {
		return nil, err
	}
	return &url, nil
}

func UpdateShortCode(id uint64, shortCode string) error {
	return db.Model(&models.URL{}).Where("id = ?", id).
		Update("short_code", shortCode).Error
}

func IncrementClickCnt(shortCode string) error {
	return db.Model(&models.URL{}).
		Where("short_code = ?", shortCode).
		UpdateColumn("click_cnt", gorm.Expr("click_cnt + 1")).Error
}

func UpdateLongURLByShort(shortCode, longCode string) error {
	return db.Model(&models.URL{}).
		Where("short_code = ?", shortCode).
		UpdateColumn("long_url", longCode).Error
}

// UpdateURL 更新短链接的 long_url 和 expire_at
// expireAt 为 nil 时保持不变，非 nil 时更新（可用于清除过期时间）
func UpdateURL(shortCode, longURL string, expireAt *time.Time) error {
	updates := map[string]any{
		"long_url":   longURL,
		"updated_at": time.Now(),
	}
	if expireAt != nil {
		updates["expire_at"] = expireAt
	}
	return db.Model(&models.URL{}).Where("short_code = ?", shortCode).Updates(updates).Error
}

func DeleteURL(userID int64, shortCode string) error {
	result := db.Where("user_id = ? AND short_code = ?", userID, shortCode).Delete(&models.URL{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetURLsByUserID 分页获取用户的短链接列表
func GetURLsByUserID(userID int64, page, pageSize int) ([]models.URL, int64, error) {
	var urls []models.URL
	var total int64

	if err := db.Model(&models.URL{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&urls).Error

	return urls, total, err
}

// GetURLByShortCodeAndUser 按短码和用户ID查询（所有权校验用）
func GetURLByShortCodeAndUser(shortCode string, userID int64) (*models.URL, error) {
	var url models.URL
	err := db.Where("short_code = ? AND user_id = ?", shortCode, userID).First(&url).Error
	if err != nil {
		return nil, err
	}
	return &url, nil
}

// DeleteURLByShortCodeAndUser 按短码和用户ID删除（所有权校验用）
func DeleteURLByShortCodeAndUser(shortCode string, userID int64) error {
	return db.Where("short_code = ? AND user_id = ?", shortCode, userID).Delete(&models.URL{}).Error
}

// DeleteURLsByUserID 删除用户的所有短链接（注销账号用）
func DeleteURLsByUserID(userID int64) error {
	return db.Where("user_id = ?", userID).Delete(&models.URL{}).Error
}

// GetAllShortCodes 获取全部短码（布隆过滤器初始化用）
func GetAllShortCodes() ([]string, error) {
	var codes []string
	err := db.Model(&models.URL{}).Pluck("short_code", &codes).Error
	return codes, err
}

// GetShortCodesByUserID 获取用户所有短码列表（注销时清缓存用）
func GetShortCodesByUserID(userID int64) ([]string, error) {
	var codes []string
	err := db.Model(&models.URL{}).Where("user_id = ?", userID).Pluck("short_code", &codes).Error
	return codes, err
}

// ─── GORM Scopes ───

// ScopePaginate 分页
func ScopePaginate(page, pageSize int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		offset := (page - 1) * pageSize
		return db.Offset(offset).Limit(pageSize)
	}
}

// ScopeKeywordSearch 关键词模糊搜索长链接（安全转义 LIKE 通配符）
func ScopeKeywordSearch(keyword string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if keyword == "" {
			return db
		}
		// 转义 LIKE 通配符，防止用户输入 % 或 _ 导致意外匹配
		escaped := strings.NewReplacer("%", "\\%", "_", "\\_", "\\", "\\\\").Replace(keyword)
		return db.Where("urls.long_url LIKE ?", "%"+escaped+"%")
	}
}

// ScopeStatusFilter 状态筛选
func ScopeStatusFilter(status string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch status {
		case "active":
			return db.Where("urls.expire_at IS NULL OR urls.expire_at > NOW()")
		case "expired":
			return db.Where("urls.expire_at IS NOT NULL AND urls.expire_at <= NOW()")
		default:
			return db
		}
	}
}

// ScopeSort 排序（白名单防注入）
func ScopeSort(sort, order string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		allowedSorts := map[string]string{
			"created_at": "urls.created_at",
			"click_cnt":  "urls.click_cnt",
		}
		col, ok := allowedSorts[sort]
		if !ok {
			col = "urls.created_at"
		}
		if order == "asc" {
			return db.Order(col + " ASC")
		}
		return db.Order(col + " DESC")
	}
}

// ─── 管理员查询 ───

// URLWithUser 关联查询结果
type URLWithUser struct {
	models.URL
	Username string `json:"username"`
}

// GetAdminURLs 管理员分页查询全部短链接（关联用户名）
func GetAdminURLs(page, pageSize int, keyword, status, sort, order string) ([]URLWithUser, int64, error) {
	var total int64
	var results []URLWithUser

	base := db.Model(&models.URL{}).
		Select("urls.*, users.username").
		Joins("LEFT JOIN users ON urls.user_id = users.id").
		Scopes(
			ScopeKeywordSearch(keyword),
			ScopeStatusFilter(status),
		)

	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := base.
		Scopes(ScopeSort(sort, order), ScopePaginate(page, pageSize)).
		Find(&results).Error

	return results, total, err
}

// URLStats 全局 URL 统计
type URLStats struct {
	TotalURLs    int64
	ActiveURLs   int64
	ExpiredURLs  int64
	TodayCreated int64
}

// GetURLStats 获取全局 URL 统计
func GetURLStats() (*URLStats, error) {
	s := &URLStats{}
	today := time.Now().Truncate(24 * time.Hour)

	if err := db.Model(&models.URL{}).Count(&s.TotalURLs).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&models.URL{}).
		Where("expire_at IS NULL OR expire_at > NOW()").Count(&s.ActiveURLs).Error; err != nil {
		return nil, err
	}
	s.ExpiredURLs = s.TotalURLs - s.ActiveURLs

	if err := db.Model(&models.URL{}).
		Where("created_at >= ?", today).Count(&s.TodayCreated).Error; err != nil {
		return nil, err
	}

	return s, nil
}

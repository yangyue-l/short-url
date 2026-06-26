package mysql

import (
	"short-url/models"

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

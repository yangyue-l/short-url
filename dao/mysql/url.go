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

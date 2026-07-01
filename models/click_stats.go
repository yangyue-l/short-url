package models

import "time"

// ClickStatsDaily 按天预聚合统计表，加速统计查询
type ClickStatsDaily struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ShortCode string    `gorm:"index:idx_code_date;type:varchar(16);not null;column:short_code" json:"short_code"`
	StatDate  string    `gorm:"index:idx_code_date;type:varchar(10);not null;column:stat_date" json:"stat_date"`
	PV        int64     `gorm:"default:0;column:pv" json:"pv"`
	UV        int64     `gorm:"default:0;column:uv" json:"uv"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (ClickStatsDaily) TableName() string {
	return "click_stats_daily"
}

// ClickLogCleanup 定时清理 90 天前的日志（只保留统计表数据）
// Usage: db.Where("created_at < ?", time.Now().AddDate(0, 0, -90)).Delete(&ClickLog{})

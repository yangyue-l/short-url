package models

import "time"

type URL struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	LongURL   string     `gorm:"type:text;not null;column:long_url" json:"long_url"`
	ShortCode string     `gorm:"type:varchar(16);uniqueIndex;column:short_code" json:"short_code"`
	ExpireAt  *time.Time `gorm:"column:expire_at" json:"expire_at,omitempty"`
	ClickCnt  int64      `gorm:"default:0;column:click_cnt" json:"click_cnt"`
	CreatedAt time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (URL) TableName() string {
	return "urls"
}

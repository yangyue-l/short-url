package models

import "time"

// ClickLog 点击日志 ORM 模型
type ClickLog struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ShortCode string    `gorm:"index;type:varchar(16);not null;column:short_code" json:"short_code"`
	IP        string    `gorm:"type:varchar(45);column:ip" json:"ip"`
	Referer   string    `gorm:"type:varchar(512);column:referer" json:"referer"`
	UserAgent string    `gorm:"type:varchar(512);column:user_agent" json:"user_agent"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (ClickLog) TableName() string {
	return "click_logs"
}

// ClickItem 点击数据项（供 MQ 消费者和 Redis 模块共享）
type ClickItem struct {
	ShortCode string
	IP        string
	Referer   string
	UserAgent string
	Timestamp int64
}

// ─── 统计相关结构体 ───

// ClicksByDay 按天统计
type ClicksByDay struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// TopSource TopN 来源/浏览器
type TopSource struct {
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

// ClickStats 点击统计聚合结果（DAO 查询返回）
type ClickStats struct {
	TotalClicks int64
	UniqueIPs   int64
	TodayClicks int64
	ClicksByDay []ClicksByDay
	TopReferers []TopSource
	TopBrowsers []TopSource
}

package models

// ShortenRequest 创建短链接请求
type ShortenRequest struct {
	LongURL  string `json:"long_url" binding:"required,url"`
	ExpireIn int64  `json:"expire_in"` // 过期时间（秒），0 表示永不过期
}

// ShortenResponse 创建短链接响应
type ShortenResponse struct {
	ShortURL  string `json:"short_url"`
	ShortCode string `json:"short_code"`
	LongURL   string `json:"long_url"`
	ExpireAt  string `json:"expire_at,omitempty"`
}

// URLInfoResponse 短链接信息响应
type URLInfoResponse struct {
	ShortCode string `json:"short_code"`
	LongURL   string `json:"long_url"`
	ClickCnt  int64  `json:"click_cnt"`
	CreatedAt string `json:"created_at"`
	ExpireAt  string `json:"expire_at,omitempty"`
}

package models

// ParamShortenRequest 创建短链接请求
type ParamShortenRequest struct {
	LongURL  string `json:"long_url" binding:"required,url"`
	ExpireIn int64  `json:"expire_in"` // 过期时间（秒），0 表示永不过期
}

// ParamShortenResponse 创建短链接响应
type ParamShortenResponse struct {
	ShortURL  string `json:"short_url"`
	ShortCode string `json:"short_code"`
	LongURL   string `json:"long_url"`
	ExpireAt  string `json:"expire_at,omitempty"`
}

// ParamURLInfoResponse 短链接信息响应
type ParamURLInfoResponse struct {
	ShortCode string `json:"short_code"`
	LongURL   string `json:"long_url"`
	ClickCnt  int64  `json:"click_cnt"`
	IsExpired bool   `json:"is_expired"`
	CreatedAt string `json:"created_at"`
	ExpireAt  string `json:"expire_at,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// ParamBatchURLRequest 批量创建短链接接收参数
type ParamBatchURLRequest struct {
	RequestID string             `json:"request_id"`
	URLs      []ParamURLsRequest `json:"urls"`
}

type ParamURLsRequest struct {
	LongURL    string `json:"long_url" binding:"required,url"`
	CustomCode string `json:"custom_code"`
	ExpireIn   int64  `json:"expire_in"`
}

// ParamBatchURLResponse 批量创建短链接返回参数
type ParamBatchURLResponse struct {
	Results      []ParamURLsResponse `json:"results"`
	Total        int                 `json:"total"`
	SuccessCount int                 `json:"success_count"`
	FailCount    int                 `json:"fail_count"`
}

type ParamURLsResponse struct {
	ShortURL  string `json:"short_url,omitempty"`
	ShortCode string `json:"short_code,omitempty"`
	LongURL   string `json:"long_url"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

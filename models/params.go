package models

// ParamShortenRequest 创建短链接请求
type ParamShortenRequest struct {
	LongURL    string `json:"long_url" binding:"required,url"`
	CustomCode string `json:"custom_code"`
	ExpireIn   int64  `json:"expire_in"`
}

// ParamShortenResponse 创建短链接响应
type ParamShortenResponse struct {
	ShortURL  string `json:"short_url"`
	ShortCode string `json:"short_code"`
	LongURL   string `json:"long_url"`
	ExpireAt  string `json:"expire_at,omitempty"`
	CreatedAt string `json:"created_at"`
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

// ParamUpdateRequest 更新短链接请求体
type ParamUpdateRequest struct {
	LongURL  string `json:"long_url" binding:"required,url"`
	ExpireIn int64  `json:"expire_in"`
}

type ParamUpdateResponse struct {
	ShortCode string `json:"short_code"`
	LongURL   string `json:"long_url"`
	ExpireAt  string `json:"expire_at,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// ---------- 用户相关参数 ----------

// ParamRegisterRequest 用户注册请求
type ParamRegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6,max=32"`
	Email    string `json:"email"`
}

// ParamRegisterResponse 用户注册响应
type ParamRegisterResponse struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
}

// ParamLoginRequest 用户登录请求
type ParamLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// ParamUserBrief 用户简要信息
type ParamUserBrief struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
}

// ParamLoginResponse 用户登录响应
type ParamLoginResponse struct {
	Token    string         `json:"token"`
	ExpireAt string         `json:"expire_at"`
	User     ParamUserBrief `json:"user"`
}

// ParamRefreshResponse 刷新 Token 响应
type ParamRefreshResponse struct {
	Token    string `json:"token"`
	ExpireAt string `json:"expire_at"`
}

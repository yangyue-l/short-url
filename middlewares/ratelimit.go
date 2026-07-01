package middlewares

import (
	"fmt"
	"net/http"
	"short-url/controller"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ─── 基于 x/time/rate 的令牌桶限流器 ───
//
// 相比手写 token bucket：
//   - x/time/rate 是 Go 官方扩展库，久经考验
//   - limiter.Reserve() 可精确获取 Retry-After 延迟
//   - 每个 visitor 持有独立的 rate.Limiter，减少全局锁竞争
//   - 通过 Stop() 优雅关闭清理协程，避免 goroutine 泄漏

type visitorEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter 按 key 维度的令牌桶限流器
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitorEntry
	r        rate.Limit    // 每秒填充 token 数
	b        int           // 桶容量（允许的突发请求数）
	maxIdle  time.Duration // 清理不活跃访问者的间隔
	done     chan struct{}
}

// NewRateLimiter 创建限流器并启动后台清理协程
func NewRateLimiter(r rate.Limit, b int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitorEntry, 1024),
		r:        r,
		b:        b,
		maxIdle:  5 * time.Minute,
		done:     make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop 优雅关闭清理协程
func (rl *RateLimiter) Stop() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	select {
	case <-rl.done:
		// 已经关闭
	default:
		close(rl.done)
	}
}

// allow 判断 key 是否允许通过，返回是否允许、重试等待时间、剩余 token 数
func (rl *RateLimiter) allow(key string) (allowed bool, retryAfter time.Duration, remaining int) {
	rl.mu.Lock()
	entry, ok := rl.visitors[key]
	if !ok {
		// 新访问者：创建独立 limiter，初始满桶
		limiter := rate.NewLimiter(rl.r, rl.b)
		_ = limiter.Reserve() // 消耗 1 token
		entry = &visitorEntry{limiter: limiter, lastSeen: time.Now()}
		rl.visitors[key] = entry
		rl.mu.Unlock()
		return true, 0, rl.b - 1
	}
	entry.lastSeen = time.Now()
	rl.mu.Unlock()

	// 对已有访问者：仅在其独立 limiter 上操作，无全局锁
	r := entry.limiter.Reserve()
	if r.OK() && r.Delay() == 0 {
		// token 立即可用
		rem := max(int(entry.limiter.Tokens()), 0)
		return true, 0, rem
	}
	// token 需要等待或不可用：取消预约并返回拒绝
	delay := r.Delay()
	r.Cancel()
	return false, delay, 0
}

// cleanup 定期清理长时间未活跃的访问者，防止内存泄漏
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.maxIdle)
	defer ticker.Stop()
	for {
		select {
		case <-rl.done:
			return
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-rl.maxIdle)
			// 仅在 map 较大时执行清理以减少持锁时间
			if len(rl.visitors) > 0 {
				for k, v := range rl.visitors {
					if v.lastSeen.Before(cutoff) {
						delete(rl.visitors, k)
					}
				}
			}
			rl.mu.Unlock()
		}
	}
}

// ─── 限流中间件构建 ───

// limiterConfig 限流器配置
type limiterConfig struct {
	rl     *RateLimiter
	getKey func(*gin.Context) string
}

// buildMiddleware 构建 Gin 限流中间件
func buildMiddleware(cfg limiterConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := cfg.getKey(c)
		allowed, retryAfter, remaining := cfg.rl.allow(key)

		// 始终返回限流信息头
		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.rl.b))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if !allowed {
			c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, controller.Response{
				Code:    controller.CodeRateLimit,
				Message: "请求频率过高",
			})
			return
		}
		c.Next()
	}
}

// ─── 全局限流器实例 ───

var (
	redirectLimiter *RateLimiter // 跳转：3000/min，突发 500
	apiIPLimiter    *RateLimiter // API IP 层：300/min，突发 100
	loginLimiter    *RateLimiter // 登录：5/min，突发 5
	userLimiter     *RateLimiter // API UserID 层：200/min，突发 100
	limiterInitOnce sync.Once
)

func initLimiters() {
	redirectLimiter = NewRateLimiter(3000.0/60.0, 500)
	apiIPLimiter = NewRateLimiter(300.0/60.0, 100)
	loginLimiter = NewRateLimiter(5.0/60.0, 5)
	userLimiter = NewRateLimiter(200.0/60.0, 100)
}

// StopLimiters 优雅关闭所有限流器的清理协程（由 main.go 调用）
func StopLimiters() {
	for _, rl := range []*RateLimiter{redirectLimiter, apiIPLimiter, loginLimiter, userLimiter} {
		if rl != nil {
			rl.Stop()
		}
	}
}

// RedirectRateLimit 跳转接口限流（IP 维度，高阈值仅防脚本刷量）
func RedirectRateLimit() gin.HandlerFunc {
	limiterInitOnce.Do(initLimiters)
	return buildMiddleware(limiterConfig{
		rl: redirectLimiter,
		getKey: func(c *gin.Context) string {
			return "ip:" + c.ClientIP()
		},
	})
}

// APIRateLimit API 接口 IP 层限流（防 DDoS，阈值宽松；JWT 之前执行）
func APIRateLimit() gin.HandlerFunc {
	limiterInitOnce.Do(initLimiters)
	return buildMiddleware(limiterConfig{
		rl: apiIPLimiter,
		getKey: func(c *gin.Context) string {
			return "ip:" + c.ClientIP()
		},
	})
}

// LoginRateLimit 登录接口限流（IP 维度，严格防撞库）
func LoginRateLimit() gin.HandlerFunc {
	limiterInitOnce.Do(initLimiters)
	return buildMiddleware(limiterConfig{
		rl: loginLimiter,
		getKey: func(c *gin.Context) string {
			return "login:" + c.ClientIP()
		},
	})
}

// UserRateLimit API 接口 UserID 层限流（JWT 之后，防单用户滥用配额）
// 每个已认证用户独立计数，避免同一 IP（如公司 NAT）下多用户互相影响
func UserRateLimit() gin.HandlerFunc {
	limiterInitOnce.Do(initLimiters)
	return buildMiddleware(limiterConfig{
		rl: userLimiter,
		getKey: func(c *gin.Context) string {
			userID, err := controller.GetCurrentUser(c)
			if err != nil {
				// 兜底：JWT 认证后理论上不会走到这里
				return "ip:" + c.ClientIP()
			}
			return fmt.Sprintf("user:%d", userID)
		},
	})
}

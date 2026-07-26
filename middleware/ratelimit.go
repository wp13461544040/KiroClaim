package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/huey1in/KiroClaim/utils"

	"github.com/gin-gonic/gin"
)

// RateLimiter 按 IP 做滑动窗口限流。
type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
	enabled  bool
}

// NewRateLimiter 创建请求限流器。
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
		enabled:  limit > 0,
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.cleanup()
		}
	}()
	return rl
}

// Update 更新限流配置。
func (rl *RateLimiter) Update(limit int, window time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limit = limit
	rl.window = window
	rl.enabled = limit > 0
}

// Allow 判断当前 IP 是否允许继续请求。
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if !rl.enabled {
		return true
	}

	now := time.Now()
	cutoff := now.Add(-rl.window)

	reqs := rl.requests[ip]
	valid := reqs[:0]
	for _, t := range reqs {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.requests[ip] = valid
		return false
	}

	rl.requests[ip] = append(valid, now)
	return true
}

// cleanup 清理过期记录。
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for ip, reqs := range rl.requests {
		valid := reqs[:0]
		for _, t := range reqs {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(rl.requests, ip)
		} else {
			rl.requests[ip] = valid
		}
	}
}

// LoginLimiter 登录失败锁定限流器。
type LoginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	limit    int
	window   time.Duration
	lockout  time.Duration
	lockouts map[string]time.Time
	enabled  bool
}

// NewLoginLimiter 创建登录失败限流器。
func NewLoginLimiter(limit int, window, lockout time.Duration) *LoginLimiter {
	ll := &LoginLimiter{
		failures: make(map[string][]time.Time),
		lockouts: make(map[string]time.Time),
		limit:    limit,
		window:   window,
		lockout:  lockout,
		enabled:  limit > 0,
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			ll.cleanup()
		}
	}()
	return ll
}

// Update 更新登录失败锁定配置。
func (ll *LoginLimiter) Update(limit int, window, lockout time.Duration) {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	ll.limit = limit
	ll.window = window
	ll.lockout = lockout
	ll.enabled = limit > 0
}

// IsLocked 判断当前 IP 是否处于锁定状态。
func (ll *LoginLimiter) IsLocked(ip string) bool {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	if !ll.enabled {
		return false
	}
	if lockUntil, ok := ll.lockouts[ip]; ok {
		if time.Now().Before(lockUntil) {
			return true
		}
		delete(ll.lockouts, ip)
	}
	return false
}

// RecordFailure 记录一次失败，返回是否触发锁定。
func (ll *LoginLimiter) RecordFailure(ip string) bool {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	if !ll.enabled {
		return false
	}
	now := time.Now()
	cutoff := now.Add(-ll.window)
	reqs := ll.failures[ip]
	valid := reqs[:0]
	for _, t := range reqs {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	valid = append(valid, now)
	ll.failures[ip] = valid

	if len(valid) >= ll.limit {
		ll.lockouts[ip] = now.Add(ll.lockout)
		ll.failures[ip] = nil
		return true
	}
	return false
}

// ResetIP 登录成功时清理失败记录。
func (ll *LoginLimiter) ResetIP(ip string) {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	delete(ll.failures, ip)
	delete(ll.lockouts, ip)
}

// cleanup 清理过期记录。
func (ll *LoginLimiter) cleanup() {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	now := time.Now()
	for ip, lockUntil := range ll.lockouts {
		if now.After(lockUntil) {
			delete(ll.lockouts, ip)
		}
	}
	cutoff := now.Add(-ll.window)
	for ip, reqs := range ll.failures {
		valid := reqs[:0]
		for _, t := range reqs {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(ll.failures, ip)
		} else {
			ll.failures[ip] = valid
		}
	}
}

// RateLimitMiddleware 创建 Gin 限流中间件。
func RateLimitMiddleware(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.Allow(ip) {
			utils.RateLimitHits.Inc()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    1,
				"message": "请求过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}

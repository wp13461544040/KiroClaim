package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 公开端点（/subtool/*、/share/*）的令牌桶限流。
//
// 后台接口靠 JWT 挡住滥用，而订阅工具端点是无鉴权的——
// 没有任何速率约束的话，一个循环就能把后端吃满。
// 实现是按 IP 的令牌桶，状态放内存：KiroClaim 是单进程，不需要跨实例共享。

const (
	// shareRateBurst 突发容量。留 30 足够覆盖正常使用与刷新。
	shareRateBurst = 30
	// shareRateRefill 每秒回填的令牌数。稳态允许约 2 QPS/IP。
	shareRateRefill = 2
	// shareVisitorTTL 空闲多久后回收访客记录，防止 map 无界增长。
	shareVisitorTTL = 10 * time.Minute
	// shareVisitorSweepEvery 清理间隔。
	shareVisitorSweepEvery = 5 * time.Minute
	// shareVisitorMax 记录数硬上限；触及时直接放行而不是继续膨胀。
	shareVisitorMax = 20000
)

type shareVisitor struct {
	tokens float64
	seen   time.Time
}

type shareRateLimiter struct {
	mu        sync.Mutex
	visitors  map[string]*shareVisitor
	lastSweep time.Time
}

func newShareRateLimiter() *shareRateLimiter {
	return &shareRateLimiter{
		visitors:  make(map[string]*shareVisitor),
		lastSweep: time.Now(),
	}
}

func (r *shareRateLimiter) allow(ip string) bool {
	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.sweepLocked(now)

	v, ok := r.visitors[ip]
	if !ok {
		if len(r.visitors) >= shareVisitorMax {
			return true // 记录已满：放行，宁可暂时失去限流
		}
		r.visitors[ip] = &shareVisitor{tokens: shareRateBurst - 1, seen: now}
		return true
	}

	v.tokens += now.Sub(v.seen).Seconds() * shareRateRefill
	if v.tokens > shareRateBurst {
		v.tokens = shareRateBurst
	}
	v.seen = now

	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

func (r *shareRateLimiter) sweepLocked(now time.Time) {
	if now.Sub(r.lastSweep) < shareVisitorSweepEvery {
		return
	}
	r.lastSweep = now
	for ip, v := range r.visitors {
		if now.Sub(v.seen) > shareVisitorTTL {
			delete(r.visitors, ip)
		}
	}
}

// ShareRateLimit 返回分享/订阅工具等公开端点的限流中间件。
func ShareRateLimit() gin.HandlerFunc {
	limiter := newShareRateLimiter()
	return func(c *gin.Context) {
		if !limiter.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    1,
				"message": "请求过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}

package middleware

import (
	"strconv"
	"sync"
	"time"

	"golang-server-starter/internal/apperr"
	"golang-server-starter/internal/config"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// gcInterval 空闲限流器回收间隔
const gcInterval = time.Minute

// RateLimit 基于客户端 IP 的令牌桶限流。
// 通过 X-RateLimit-* 响应头回传限流信息，超限返回 429。
func RateLimit(cfg config.RateLimit) gin.HandlerFunc {
	store := newLimiterStore(cfg.RPS, cfg.Burst)

	return func(c *gin.Context) {
		v := store.visitor(c.ClientIP())
		allowed, remaining := v.allow()

		// 直接写 Header map，避免 http.Header 规范化把 X-RateLimit-* 改成 X-Ratelimit-*
		c.Writer.Header()["X-RateLimit-Limit"] = []string{strconv.FormatFloat(cfg.RPS, 'f', -1, 64)}
		c.Writer.Header()["X-RateLimit-Remaining"] = []string{strconv.Itoa(remaining)}
		if !allowed {
			panic(apperr.ErrTooManyRequests)
		}
		c.Next()
	}
}

type visitor struct {
	// mu 串行化 limiter 的「消费 + 读取余额」。
	// rate.Limiter.Tokens() 文档明确标注「非并发安全」，同一 IP 的并发请求
	// 若不加锁直接读 Tokens() 会构成数据竞争（-race 可检出）。
	mu       sync.Mutex
	limiter  *rate.Limiter
	lastSeen time.Time
}

// allow 消费一个令牌，并返回消费后的剩余额度。
//
// 注意顺序：必须先 Allow() 再读 Tokens()。
// 反过来的话响应头会报出「消费前」的余额，永远比真实值多 1。
func (v *visitor) allow() (allowed bool, remaining int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	allowed = v.limiter.Allow()
	return allowed, int(v.limiter.Tokens())
}

type limiterStore struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    rate.Limit
	burst    int
	lastGC   time.Time
}

func newLimiterStore(rps float64, burst int) *limiterStore {
	if rps <= 0 {
		rps = 50
	}
	if burst <= 0 {
		burst = int(rps)
	}
	return &limiterStore{
		visitors: make(map[string]*visitor),
		limit:    rate.Limit(rps),
		burst:    burst,
		lastGC:   time.Now(),
	}
}

func (s *limiterStore) visitor(key string) *visitor {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if now.Sub(s.lastGC) > gcInterval {
		s.gcLocked(now)
	}

	item, ok := s.visitors[key]
	if !ok {
		item = &visitor{limiter: rate.NewLimiter(s.limit, s.burst)}
		s.visitors[key] = item
	}
	item.lastSeen = now
	return item
}

// gcLocked 回收长时间未活跃的限流器，调用方需持有锁
func (s *limiterStore) gcLocked(now time.Time) {
	for key, item := range s.visitors {
		if now.Sub(item.lastSeen) > 3*gcInterval {
			delete(s.visitors, key)
		}
	}
	s.lastGC = now
}

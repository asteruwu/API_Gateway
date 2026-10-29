package bucket

import (
	"math"
	"sync"
	"time"

	gconst "API_Gateway/pkg/constant"
)

type TokenBucket struct {
	mu     sync.Mutex
	qps    float64
	burst  float64
	tokens float64
	last   time.Time
}

func NewTokenBucket(qps float64, burst int, now time.Time) *TokenBucket {
	return &TokenBucket{
		qps:    qps,
		burst:  float64(burst),
		tokens: float64(burst),
		last:   now,
	}
}

func (b *TokenBucket) Allow(now time.Time) (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.refill(now)

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, durationFromSeconds((1 - b.tokens) / b.qps)
}

func (b *TokenBucket) refill(now time.Time) {
	if !now.After(b.last) {
		return
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = math.Min(b.burst, b.tokens+elapsed*b.qps)
	b.last = now
}

func durationFromSeconds(seconds float64) time.Duration {
	if seconds <= 0 {
		return 0
	}
	ns := math.Ceil(seconds * float64(time.Second))
	if ns >= float64(gconst.RateLimitMaxRetryAfter) {
		return gconst.RateLimitMaxRetryAfter
	}
	return time.Duration(ns)
}

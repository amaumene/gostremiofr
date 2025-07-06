package ratelimiter

import (
	"sync"
	"time"
)

type RateLimiter interface {
	TakeToken() bool
	Wait()
}

type TokenBucket struct {
	capacity   int64
	tokens     int64
	refillRate int64
	lastRefill time.Time
	mu         sync.Mutex
}

func NewTokenBucket(capacity, refillRate int64) *TokenBucket {
	// Ensure positive values to prevent issues
	if capacity <= 0 {
		capacity = 1
	}
	if refillRate <= 0 {
		refillRate = 1
	}
	
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) TakeToken() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	
	tokensToAdd := int64(elapsed.Seconds()) * tb.refillRate
	tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
	tb.lastRefill = now

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

func (tb *TokenBucket) Wait() {
	// Calculate wait time based on refill rate
	waitTime := time.Second / time.Duration(tb.refillRate)
	if waitTime < 100*time.Millisecond {
		waitTime = 100 * time.Millisecond
	}
	
	for !tb.TakeToken() {
		time.Sleep(waitTime)
	}
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
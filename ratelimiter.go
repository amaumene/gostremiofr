package main

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	capacity   int64
	tokens     int64
	refillRate int64
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(capacity, refillRate int64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// TakeToken attempts to take a token from the bucket
// Returns true if successful, false if rate limited
func (tb *TokenBucket) TakeToken() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	
	// Refill tokens based on elapsed time
	tokensToAdd := int64(elapsed.Seconds()) * tb.refillRate
	tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
	tb.lastRefill = now

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// Rate limiters for different APIs
var (
	yggRateLimiter       = NewTokenBucket(10, 2)  // 10 requests burst, 2/second refill
	sharewoodRateLimiter = NewTokenBucket(5, 1)   // 5 requests burst, 1/second refill
	tmdbRateLimiter      = NewTokenBucket(20, 5)  // 20 requests burst, 5/second refill
	allDebridRateLimiter = NewTokenBucket(15, 3)  // 15 requests burst, 3/second refill
)

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
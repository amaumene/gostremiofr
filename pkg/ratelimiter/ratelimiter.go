// Package ratelimiter provides token bucket based rate limiting functionality.
package ratelimiter

import (
	"fmt"
	"sync"
	"time"
)

// RateLimiter defines the interface for rate limiting implementations.
type RateLimiter interface {
	// TakeToken attempts to consume a token, returns true if successful
	TakeToken() bool
	// Wait blocks until a token is available
	Wait()
	// WaitWithTimeout blocks until a token is available or timeout occurs
	WaitWithTimeout(timeout time.Duration) error
}

// TokenBucket implements the token bucket algorithm for rate limiting.
// It refills tokens at a constant rate up to a maximum capacity.
type TokenBucket struct {
	capacity   int64      // Maximum number of tokens
	tokens     int64      // Current number of available tokens
	refillRate int64      // Tokens added per second
	lastRefill time.Time  // Last time tokens were refilled
	mu         sync.Mutex // Protects concurrent access
}

// NewTokenBucket creates a new token bucket with the specified capacity and refill rate.
// capacity: maximum number of tokens the bucket can hold
// refillRate: number of tokens added per second
// Both values are normalized to at least 1 to prevent invalid configurations.
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

// TakeToken attempts to consume a token from the bucket.
// Returns true if a token was available and consumed, false otherwise.
// This method is thread-safe and refills tokens based on elapsed time.
func (tb *TokenBucket) TakeToken() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	// Calculate and add new tokens based on elapsed time
	tokensToAdd := int64(elapsed.Seconds()) * tb.refillRate
	tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
	tb.lastRefill = now

	// Try to consume a token
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// Wait blocks until a token is available.
// Uses a default timeout of 5 seconds for backward compatibility.
func (tb *TokenBucket) Wait() {
	_ = tb.WaitWithTimeout(5 * time.Second)
}

const minWaitTime = 100 * time.Millisecond

// WaitWithTimeout blocks until a token is available or the timeout expires.
// Returns nil if a token was acquired, or an error if the timeout was reached.
func (tb *TokenBucket) WaitWithTimeout(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	// Calculate wait time based on refill rate
	waitTime := time.Second / time.Duration(tb.refillRate)
	if waitTime < minWaitTime {
		waitTime = minWaitTime
	}

	for !tb.TakeToken() {
		if time.Now().After(deadline) {
			return fmt.Errorf("rate limiter timeout after %v", timeout)
		}
		time.Sleep(waitTime)
	}
	return nil
}

// min returns the smaller of two int64 values
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

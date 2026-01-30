package main

import (
	"errors"
	"sync"
	"time"
)

// --------------Token Store Definitions --------------
type TokenStore interface {
	Debit(count int) bool
	Len() int
	Cap() int
	AddTokens(count int)
}

type MemoryBucket struct {
	count int
	cap   int
	mu    sync.RWMutex
}

func NewMemoryBucket(count int, cap int) (*MemoryBucket, error) {

	if cap <= 0 {
		return nil, errors.New("Capacity must be a non-zero positive integer.")
	}

	if count < 0 {
		return nil, errors.New("count must be a non-negative integer if provided.")
	}

	if count > cap {
		return nil, errors.New("count must be less than or equal to capacity if provided.")
	}

	bucket := MemoryBucket{count: count, cap: cap}
	return &bucket, nil
}

func (b *MemoryBucket) AddTokens(count int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.count+count >= b.cap {
		b.count = b.cap
	} else {
		b.count += count
	}
}

func (b *MemoryBucket) Debit(count int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.count >= count {
		b.count -= count
		return true
	}
	return false
}

func (b *MemoryBucket) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}

func (b *MemoryBucket) Cap() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cap
}

// ----------------Limiter definition-----------
type GlobalRateLimiter struct {
	bucket TokenStore
	rate   int //token refill rate per second
	done   chan struct{}
}

func newGlobalRateLimiter(rate int, store TokenStore) *GlobalRateLimiter {

	done := make(chan struct{})
	limiter := GlobalRateLimiter{store, rate, done}
	go limiter.AddTokens()
	return &limiter
}

func (l *GlobalRateLimiter) Allow(size int) bool {
	return l.bucket.Debit(size)
}

func (l *GlobalRateLimiter) Offline() {
	close(l.done)
}

func (l *GlobalRateLimiter) AddTokens() {
	ticker := time.NewTicker(time.Second / time.Duration(l.rate))
	for {
		select {
		case <-ticker.C:
			l.bucket.AddTokens(1)
		case <-l.done:
			ticker.Stop()
			return
		}
	}
}

func NewGlobalRateLimiter(storageType StorageType, count, cap, rate int) (*GlobalRateLimiter, error) {
	var limiter *GlobalRateLimiter

	switch storageType {
	case InMemory:
		// initialize bucket
		bucket, err := NewMemoryBucket(count, cap)
		if err != nil {
			return nil, err
		}
		limiter = newGlobalRateLimiter(rate, bucket)

	case Redis:
		return nil, errors.New("Redis storage not yet implemented")

	default:
		return nil, errors.New("Unknown storage type provided.")
	}

	return limiter, nil
}

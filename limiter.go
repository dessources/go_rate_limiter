package go_rate_limiter

import (
	"errors"
	"time"
)

// --------------Token Store Definitions --------------
type TokenStore interface {
	Debit(count int) bool
	Count() int
	Capacity() int
	AddTokens(count int)
}

type MemoryBucket struct {
	count    int
	capacity int
}

func NewMemoryBucket(count int, capacity int) (*MemoryBucket, error) {

	if capacity <= 0 {
		return nil, errors.New("Capacity must be a non-zero positive integer.")
	}

	if count < 0 {
		return nil, errors.New("count must be a non-negative integer if provided.")
	}

	bucket := MemoryBucket{count, capacity}
	return &bucket, nil
}

func (b *MemoryBucket) AddTokens(count int) {
	if b.count+count >= b.capacity {
		b.count = b.capacity
	} else {
		b.count += count
	}
}

func (b *MemoryBucket) Debit(count int) bool {
	if b.count >= count {
		b.count -= count
		return true
	}
	return false
}

func (b *MemoryBucket) Count() int {
	return b.count
}

func (b *MemoryBucket) Capacity() int {
	return b.capacity
}

// ----------------Limiter definition-----------
type Limiter struct {
	bucket          TokenStore
	rate            int       //token refill rate per second
	lastRequestTime time.Time //not sure what type to use here
}

func NewLimiter(rate int, store TokenStore) *Limiter {
	limiter := Limiter{store, rate, time.Now()}

	return &limiter
}

func (l *Limiter) FillBucket() {
	elapsed := time.Since(l.lastRequestTime).Milliseconds()
	tokenCount := (elapsed * int64(l.rate)) / 1000
	l.bucket.AddTokens(int(tokenCount))
}

func (l *Limiter) Allow(size int) bool {
	l.FillBucket()
	if l.bucket.Debit(size) {
		l.lastRequestTime = time.Now()
		return true
	}
	return false
}

package main

import (
	"errors"
	"sync"
	"time"
)

const ttl = time.Minute * 30

type TimeLogStore interface {
	Add(key string, window time.Duration) error
	Remove(key string) error
	Clean() error

	Cap() int
	Len() int
}

type InMemoryTimeLogStore struct {
	cap   int
	len   int
	limit int
	logs  map[string][]time.Time
	mu    sync.RWMutex
}

func (s *InMemoryTimeLogStore) Add(k string, w time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	//if new client, check global capacity
	if _, exists := s.logs[k]; exists {

		// remove old
		//delete entry if all logs old
		var lastLog time.Time
		if lastLogIndex := len(s.logs[k]) - 1; lastLogIndex >= 0 {
			lastLog = s.logs[k][len(s.logs[k])-1]
			if time.Since(lastLog) >= w {
				delete(s.logs, k)
				s.len--
			} else {
				//find first log within window and resize slize
				for i, logTime := range s.logs[k] {
					if time.Since(logTime) < w {
						s.logs[k] = s.logs[k][i:]

						break
					}
				}
			}
		}

	} else {
		if s.len >= s.cap {
			return errors.New("Storage is at capacity.")
		}

	}

	// check rate limit
	if _, exists := s.logs[k]; !exists {
		s.logs[k] = make([]time.Time, 0)
		s.len++
	} else if len(s.logs[k]) >= s.limit {
		return errors.New("Rate limit exceeded. Please try again later")
	}

	//add log
	s.logs[k] = append(s.logs[k], time.Now())
	return nil

}

func (s *InMemoryTimeLogStore) Remove(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.logs[k]; !exists {
		return errors.New("Entry not found")
	}
	delete(s.logs, k)
	s.len--
	return nil

}

func (s *InMemoryTimeLogStore) Clean() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	keysToDelete := []string{}
	for key, val := range s.logs {
		if len(val) > 0 {
			lastRequestTime := val[len(val)-1]
			if time.Since(lastRequestTime) > ttl {
				keysToDelete = append(keysToDelete, key)
			}
		}
	}

	for _, key := range keysToDelete {
		delete(s.logs, key)
		s.len--
	}
	return nil
}

func (s *InMemoryTimeLogStore) Cap() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cap
}

func (s *InMemoryTimeLogStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.len
}

func (l *PerClientLimiter) RegularlyRemoveOldClients() {
	ticker := time.NewTicker(time.Minute * 30)
	for {
		select {
		case <-ticker.C:
			l.timeLogStore.Clean()

		case <-l.done:
			ticker.Stop()
			return
		}
	}
}

type PerClientLimiter struct {
	timeLogStore TimeLogStore
	window       time.Duration
	done         chan struct{}
}

func (l *PerClientLimiter) Allow(clientID string) error {
	return l.timeLogStore.Add(clientID, l.window)
}

func (l *PerClientLimiter) Offline() {
	close(l.done)
}

func NewPerClientLimiter(storateType StorageType, cap int, limit int, window time.Duration) (*PerClientLimiter, error) {

	var limiter PerClientLimiter
	switch storateType {
	case InMemory:
		logs := make(map[string][]time.Time)
		done := make(chan struct{})
		timeLogStore := &InMemoryTimeLogStore{cap: cap, logs: logs, limit: limit}
		limiter = PerClientLimiter{timeLogStore, window, done}

		go limiter.RegularlyRemoveOldClients()

		return &limiter, nil
	case Redis:
		return nil, errors.New("Redis storage not yet implemented")

	default:
		return nil, errors.New("Unknown storage type provided.")
	}
}

package main

import (
	"errors"
	"sync"
	"time"
)

const ttl = time.Minute * 30

type TimeLogStore interface {
	Add(key string, window time.Duration) error
	RemoveClient(key string) error
	RemoveInactiveClients() error
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

	if _, exists := s.logs[k]; exists {
		// if existing client, remove old logs outside window
		s.RemoveOldLogs(k, w)

	} else {
		//if new client, check global capacity
		if s.len >= s.cap {
			return errors.New("Storage is at capacity.")
		}
		s.logs[k] = make([]time.Time, 0, s.limit)
		s.len++
	}

	if len(s.logs[k]) >= s.limit {
		// check rate limit
		return errors.New("Rate limit exceeded. Please try again later")
	}

	//add log entry
	s.logs[k] = append(s.logs[k], time.Now())
	return nil
}

// RemoveOldLogs assumes caller holds s.mu.Lock()
func (s *InMemoryTimeLogStore) RemoveOldLogs(k string, w time.Duration) {

	if lastLogIndex := len(s.logs[k]) - 1; lastLogIndex >= 0 {
		lastLog := s.logs[k][lastLogIndex]
		if time.Since(lastLog) >= w {
			// All logs are old, reset the slice
			s.logs[k] = s.logs[k][:0]
		} else {
			//find first log within window and resize slice
			for i, logTime := range s.logs[k] {
				if time.Since(logTime) < w {
					s.logs[k] = s.logs[k][i:]
					break
				}
			}
		}
	}
}

func (s *InMemoryTimeLogStore) RemoveClient(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.logs[k]; !exists {
		return errors.New("Entry not found")
	}
	delete(s.logs, k)
	s.len--
	return nil
}

func (s *InMemoryTimeLogStore) RemoveInactiveClients() error {
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

func (l *PerClientLimiter) RemoveInactiveClientsRoutine() {
	ticker := time.NewTicker(time.Minute * 30)
	for {
		select {
		case <-ticker.C:
			l.timeLogStore.RemoveInactiveClients()

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

		go limiter.RemoveInactiveClientsRoutine()

		return &limiter, nil
	case Redis:
		return nil, errors.New("Redis storage not yet implemented")

	default:
		return nil, errors.New("Unknown storage type provided.")
	}
}

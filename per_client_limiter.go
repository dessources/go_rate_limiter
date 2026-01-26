package main

import (
	"errors"
	"sync"
	"time"
)

const ttl = time.Minute * 30

type TimeLogStore interface {
	Add(key string, time time.Time) error
	Remove(key string) error
	RemoveOld(key string, window time.Duration) error
	Clean() error
	GetCount(key string) (int, error)
	//GetAll() (map[string][]time.Time, error)

	Cap() int
	Len() int
}

type InMemoryTimeLogStore struct {
	cap  int
	len  int
	logs map[string][]time.Time
	mu   sync.RWMutex
}

func (s *InMemoryTimeLogStore) Add(k string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.logs[k]) == 0 {
		if s.len >= s.cap {
			return errors.New("Storage is at capacity.")
		}
		s.len++
	}

	s.logs[k] = append(s.logs[k], t)

	return nil

}

func (s *InMemoryTimeLogStore) Remove(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.logs[k]) > 0 {
		delete(s.logs, k)
		s.len--
		return nil
	}
	return errors.New("Entry not found")
}

func (s *InMemoryTimeLogStore) RemoveOld(k string, window time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, logTime := range s.logs[k] {
		if time.Since(logTime) < window {
			s.logs[k] = s.logs[k][i:]
			return nil
		}
	}
	//if we reached this part all logs are older than window so we remove the whole entry
	if len(s.logs[k]) > 0 {
		s.len--
		delete(s.logs, k)
	}

	return nil
}

func (s *InMemoryTimeLogStore) Clean() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, val := range s.logs {
		if len(val) > 0 {
			lastRequestTime := val[len(val)-1]
			if time.Since(lastRequestTime) > ttl {
				delete(s.logs, key)
				s.len--
			}
		}
	}
	return nil
}

func (s *InMemoryTimeLogStore) GetCount(k string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.logs[k]), nil
}

// func (s *InMemoryTimeLogStore) GetAll() (map[string][]time.Time, error) {
// 	s.mu.RLock()
// 	defer s.mu.RUnlock()
// 	return s.logs, nil
// }

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
	limit        int
	done         chan struct{}
}

func (l *PerClientLimiter) Allow(clientID string) error {
	store := l.timeLogStore

	if err := store.RemoveOld(clientID, l.window); err != nil {
		return errors.New("An error was encountered while removing old request logs")
	}

	count, err := store.GetCount(clientID)
	if err != nil {
		return errors.New("An error was encountered while reading request counts")
	}

	if count >= l.limit {
		return errors.New("Rate limit exceeded. Please try again later")
	} else {
		if err := store.Add(clientID, time.Now()); err != nil {
			return err
		}
		return nil
	}
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
		timeLogStore := &InMemoryTimeLogStore{cap: cap, logs: logs}
		limiter = PerClientLimiter{timeLogStore, window, limit, done}

		go limiter.RegularlyRemoveOldClients()

		return &limiter, nil
	case Redis:
		return nil, errors.New("Redis storage not yet implemented")

	default:
		return nil, errors.New("Unknown storage type provided.")
	}
}

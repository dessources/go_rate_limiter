package main

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
)

const (
	MinAge     = time.Duration(time.Minute * 30)
	MinCap int = 10
)

type UrlShortener interface {
	AddMapping(original, short string) (bool, error)
	RetrieveUrl(short string) (string, error)
	RemoveMapping(short string) error
	RegularlyResetMappings()
	Offline()
	Cap() int
	Len() int
}

type UrlMapping struct {
	originalUrl string
	createdAt   time.Time
}

type InMemoryUrlMap struct {
	cap     int
	len     int
	mapping map[string]*UrlMapping
	done    chan struct{}
	ttl     time.Duration
	mu      sync.RWMutex
}

func (m *InMemoryUrlMap) AddMapping(original, short string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.len < m.cap {
		if m.mapping[short] != nil {
			return true, nil
		}
		m.len++
		m.mapping[short] = &UrlMapping{original, time.Now()}
		return false, nil
	} else {
		return false, errors.New("Url map is full.")
	}
}

func (m *InMemoryUrlMap) RetrieveUrl(s string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if mapping := m.mapping[s]; mapping != nil {
		return mapping.originalUrl, nil
	} else {
		return "", errors.New("Url not found")
	}
}

func (m *InMemoryUrlMap) RemoveMapping(short string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mapping[short] != nil {
		delete(m.mapping, short)
		m.len--
		return nil
	}

	return errors.New("Url not found")
}

func (m *InMemoryUrlMap) RegularlyResetMappings() {
	ticker := time.NewTicker(m.ttl / 2)
	for {
		select {
		case <-ticker.C:
			m.mu.Lock()

			for key, val := range m.mapping {
				if time.Since(val.createdAt) > m.ttl {

					delete(m.mapping, key)
					m.len--
				}
			}
			m.mu.Unlock()
		case <-m.done:
			ticker.Stop()
			return
		}
	}
}

func (m *InMemoryUrlMap) Cap() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cap
}
func (m *InMemoryUrlMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.len
}

func (m *InMemoryUrlMap) Offline() {
	close(m.done)
}

func NewUrlShortener(storageType StorageType, cap int, ttl time.Duration) (UrlShortener, error) {
	if cap < MinCap {
		return nil, fmt.Errorf("Capacity has to be at least %d", MinCap)
	}
	if ttl < MinAge {
		return nil, fmt.Errorf("Time to live has to be at least %d", MinAge)
	}

	var urlShortener UrlShortener

	switch storageType {
	case InMemory:
		done := make(chan struct{})
		mapping := make(map[string]*UrlMapping)
		urlShortener = &InMemoryUrlMap{cap: cap, done: done, ttl: ttl, mapping: mapping}

		// reset mappings every hour
		go urlShortener.RegularlyResetMappings()
		return urlShortener, nil
	case Redis:
		return nil, errors.New("Redis storage not yet implemented")
	}

	return nil, errors.New("Unknown error in initializing url shortener.")
}

//--------------------------------------------------------------------------
// Shorten functionality definition
//-------------------------------------------------------------------------

const ShortUrlLength int = 10

var charTypes = [3]rune{48, 65, 97} // ascii start value for numbers, upper & lower letters

func Shorten(s UrlShortener, original string) (string, error) {

	var shortUrl string
	var assumeCollision = true

	for assumeCollision {
		var result strings.Builder
		var charPos int
		var charType int
		for range ShortUrlLength {

			if charType = rand.IntN(3); charType == 0 {
				charPos = rand.IntN(10)
			} else {
				charPos = rand.IntN(26)
			}

			char := charTypes[charType] + rune(charPos)
			result.WriteRune(char)
		}

		shortUrl = result.String()
		if match, err := s.AddMapping(original, shortUrl); err != nil {
			return "", err
		} else if !match {
			assumeCollision = false
		}
	}

	return shortUrl, nil
}

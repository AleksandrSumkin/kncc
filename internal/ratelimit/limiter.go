package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements a sliding-window rate limiter per user/key.
type Limiter struct {
	mu           sync.Mutex
	ratePerMin   int
	window       time.Duration
	requests     map[string][]time.Time
	stopCleaner  chan struct{}
}

// NewLimiter creates a rate limiter with the specified maximum requests per minute.
func NewLimiter(ratePerMinute int) *Limiter {
	if ratePerMinute <= 0 {
		ratePerMinute = 30
	}
	l := &Limiter{
		ratePerMin:  ratePerMinute,
		window:      time.Minute,
		requests:    make(map[string][]time.Time),
		stopCleaner: make(chan struct{}),
	}
	go l.cleanupLoop()
	return l
}

// Allow reports whether a request for the given key is allowed under the rate limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Filter out expired timestamps
	var valid []time.Time
	for _, t := range l.requests[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= l.ratePerMin {
		l.requests[key] = valid
		return false
	}

	valid = append(valid, now)
	l.requests[key] = valid
	return true
}

// Stop terminates the background cleanup goroutine.
func (l *Limiter) Stop() {
	close(l.stopCleaner)
}

func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-l.window)
			for k, timestamps := range l.requests {
				var valid []time.Time
				for _, t := range timestamps {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(l.requests, k)
				} else {
					l.requests[k] = valid
				}
			}
			l.mu.Unlock()
		case <-l.stopCleaner:
			return
		}
	}
}

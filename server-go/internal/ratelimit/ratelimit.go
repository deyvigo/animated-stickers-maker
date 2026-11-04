// Package ratelimit is a small in-memory sliding-window limiter keyed by
// device id + IP. It is abuse mitigation for an account-less service, not a
// security boundary; it resets when the process restarts.
package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu        sync.Mutex
	hits      map[string][]time.Time
	lastSweep time.Time
}

// New returns a limiter allowing `limit` requests per hour per key.
// limit <= 0 disables limiting.
func New(limitPerHour int) *Limiter {
	return newLimiter(limitPerHour, time.Hour, time.Now)
}

func newLimiter(limit int, window time.Duration, now func() time.Time) *Limiter {
	return &Limiter{limit: limit, window: window, now: now, hits: map[string][]time.Time{}}
}

// Allow reports whether the request is allowed, recording it if so.
func (l *Limiter) Allow(key string) bool {
	if l.limit <= 0 {
		return true
	}
	now := l.now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(now, cutoff)

	hits := l.hits[key]
	i := 0
	for i < len(hits) && !hits[i].After(cutoff) {
		i++
	}
	hits = hits[i:]
	if len(hits) >= l.limit {
		l.hits[key] = hits
		return false
	}
	l.hits[key] = append(hits, now)
	return true
}

// sweep drops keys with no recent hits, at most once per window, so the
// map can't grow without bound.
func (l *Limiter) sweep(now, cutoff time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for k, hits := range l.hits {
		if len(hits) == 0 || !hits[len(hits)-1].After(cutoff) {
			delete(l.hits, k)
		}
	}
}

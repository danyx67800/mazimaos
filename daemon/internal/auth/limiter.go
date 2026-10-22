package auth

import (
	"sync"
	"time"
)

type LimitState struct {
	Failures    int
	LockedUntil time.Time
	UpdatedAt   time.Time
}

type Limiter struct {
	mu          sync.Mutex
	entries     map[string]*LimitState
	now         func() time.Time
	maxFailures int
	baseLock    time.Duration
	maxLock     time.Duration
}

func NewLimiter() *Limiter {
	return &Limiter{
		entries:     map[string]*LimitState{},
		now:         time.Now,
		maxFailures: 5,
		baseLock:    30 * time.Second,
		maxLock:     time.Hour,
	}
}

func (l *Limiter) Allowed(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupLocked()
	e, ok := l.entries[key]
	if !ok {
		return true, 0
	}
	now := l.now()
	if now.Before(e.LockedUntil) {
		return false, e.LockedUntil.Sub(now)
	}
	return true, 0
}

func (l *Limiter) Failure(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	e, ok := l.entries[key]
	if !ok {
		e = &LimitState{}
		l.entries[key] = e
	}
	e.Failures++
	e.UpdatedAt = now
	if e.Failures >= l.maxFailures {
		exp := e.Failures - l.maxFailures
		d := l.baseLock << min(exp, 8)
		if d > l.maxLock {
			d = l.maxLock
		}
		e.LockedUntil = now.Add(d)
		return d
	}
	d := time.Duration(e.Failures) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

func (l *Limiter) Success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func (l *Limiter) cleanupLocked() {
	if len(l.entries) < 1024 {
		return
	}
	cutoff := l.now().Add(-24 * time.Hour)
	for k, e := range l.entries {
		if e.UpdatedAt.Before(cutoff) && e.LockedUntil.Before(l.now()) {
			delete(l.entries, k)
		}
	}
}

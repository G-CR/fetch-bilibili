package worker

import (
	"context"
	"sync"
	"time"
)

type limiter struct {
	interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

func newLimiter(qps int) *limiter {
	if qps <= 0 {
		return nil
	}
	return &limiter{interval: time.Second / time.Duration(qps)}
}

func (l *limiter) Wait(ctx context.Context) error {
	if l == nil || l.interval <= 0 {
		return nil
	}
	l.mu.Lock()
	now := time.Now()
	next := l.last.Add(l.interval)
	wait := next.Sub(now)
	if wait < 0 {
		wait = 0
	}
	l.last = now.Add(wait)
	l.mu.Unlock()

	if wait == 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

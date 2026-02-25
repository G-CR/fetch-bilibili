package worker

import (
	"context"
	"testing"
	"time"
)

func TestLimiterWaitCanceled(t *testing.T) {
	lim := newLimiter(1)
	lim.last = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lim.Wait(ctx); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLimiterWaitNoDelay(t *testing.T) {
	lim := newLimiter(1)
	lim.last = time.Now().Add(-2 * time.Second)
	if err := lim.Wait(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

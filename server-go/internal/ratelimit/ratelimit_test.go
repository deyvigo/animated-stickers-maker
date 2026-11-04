package ratelimit

import (
	"testing"
	"time"
)

func TestWindowAndKeys(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := newLimiter(2, time.Hour, func() time.Time { return now })

	if !l.Allow("a") || !l.Allow("a") {
		t.Fatal("first two requests must pass")
	}
	if l.Allow("a") {
		t.Fatal("third request must be limited")
	}
	if !l.Allow("b") {
		t.Fatal("other keys are independent")
	}
	now = now.Add(61 * time.Minute)
	if !l.Allow("a") {
		t.Fatal("window should have expired")
	}
}

func TestDisabledWhenLimitNotPositive(t *testing.T) {
	l := New(0)
	for i := 0; i < 1000; i++ {
		if !l.Allow("x") {
			t.Fatal("disabled limiter must always allow")
		}
	}
}

func TestSweepRemovesIdleKeys(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := newLimiter(5, time.Hour, func() time.Time { return now })
	l.Allow("idle")
	now = now.Add(3 * time.Hour)
	l.Allow("fresh")
	if _, ok := l.hits["idle"]; ok {
		t.Fatal("idle key should have been swept")
	}
}

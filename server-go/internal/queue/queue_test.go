package queue

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundsConcurrency(t *testing.T) {
	q := New(2, 1)
	var cur, peak atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.RunDownload(context.Background(), func(context.Context) {
				n := cur.Add(1)
				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				cur.Add(-1)
			})
		}()
	}
	wg.Wait()
	if peak.Load() != 2 {
		t.Fatalf("peak concurrency = %d, want 2", peak.Load())
	}
}

func TestCancelWhileWaiting(t *testing.T) {
	q := New(1, 1)
	release := make(chan struct{})
	started := make(chan struct{})
	go q.RunRender(context.Background(), func(context.Context) { close(started); <-release })
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ran := false
	err := q.RunRender(ctx, func(context.Context) { ran = true })
	close(release)
	if err == nil || ran {
		t.Fatalf("err=%v ran=%v; want cancellation without running", err, ran)
	}
}

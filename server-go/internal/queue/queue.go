// Package queue bounds concurrency for the two expensive operations:
// downloads and ffmpeg renders. A process-local semaphore per kind is
// enough for a single-instance server.
package queue

import "context"

type Queues struct {
	downloads chan struct{}
	renders   chan struct{}
}

func New(maxDownloads, maxRenders int) *Queues {
	return &Queues{
		downloads: make(chan struct{}, maxDownloads),
		renders:   make(chan struct{}, maxRenders),
	}
}

func run(ctx context.Context, sem chan struct{}, fn func(context.Context)) error {
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-sem }()
	fn(ctx)
	return nil
}

// RunDownload waits for a download slot, then runs fn. It returns ctx's
// error without running fn if ctx ends while waiting.
func (q *Queues) RunDownload(ctx context.Context, fn func(context.Context)) error {
	return run(ctx, q.downloads, fn)
}

// RunRender is RunDownload for the render slots.
func (q *Queues) RunRender(ctx context.Context, fn func(context.Context)) error {
	return run(ctx, q.renders, fn)
}

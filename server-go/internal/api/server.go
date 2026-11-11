// Package api is the HTTP layer: routes, request validation, and the
// background job/render pipelines.
package api

import (
	"context"
	"net"
	"net/http"
	"sync"

	"stickers-server/internal/apperr"
	"stickers-server/internal/config"
	"stickers-server/internal/download"
	"stickers-server/internal/models"
	"stickers-server/internal/queue"
	"stickers-server/internal/ratelimit"
	"stickers-server/internal/sticker"
	"stickers-server/internal/storage"
)

// Renderer produces the sticker files. *sticker.Fitter implements it; tests
// substitute a fake so the API can be exercised without libwebp.
type Renderer interface {
	FitAnimated(ctx context.Context, source, out string, segments []models.Segment,
		crop sticker.CropPixels, srcW, srcH int, fill sticker.PaddingFill, speed float64) (models.RenderFitInfo, error)
	FitStatic(ctx context.Context, source, out string, frameAt float64,
		crop sticker.CropPixels, srcW, srcH int, fill sticker.PaddingFill) (models.RenderFitInfo, error)
	RenderTray(ctx context.Context, source, out string, frameAt float64,
		crop sticker.CropPixels, srcW, srcH int, fill sticker.PaddingFill) error
}

type Server struct {
	Settings   config.Settings
	Store      *storage.Store
	Queues     *queue.Queues
	Limiter    *ratelimit.Limiter
	Downloader download.Downloader
	Renderer   Renderer

	// baseCtx outlives individual requests: jobs keep running after the
	// POST that created them returns. It is cancelled on shutdown.
	baseCtx context.Context
	wg      sync.WaitGroup
}

// New builds a Server. Background work runs under ctx.
func New(ctx context.Context, s config.Settings, store *storage.Store, d download.Downloader, r Renderer) *Server {
	return &Server{
		Settings:   s,
		Store:      store,
		Queues:     queue.New(s.MaxConcurrentDownloads, s.MaxConcurrentRenders),
		Limiter:    ratelimit.New(s.RateLimitPerHour),
		Downloader: d,
		Renderer:   r,
		baseCtx:    ctx,
	}
}

// Wait blocks until in-flight background jobs finish.
func (s *Server) Wait() { s.wg.Wait() }

func (s *Server) spawn(fn func(ctx context.Context)) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		fn(s.baseCtx)
	}()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"message": "Hello World"})
	})
	mux.HandleFunc("POST /api/v1/jobs", s.createJob)
	mux.HandleFunc("GET /api/v1/jobs/{job_id}", s.getJob)
	mux.HandleFunc("GET /api/v1/jobs/{job_id}/proxy.mp4", s.getProxy)
	mux.HandleFunc("GET /api/v1/jobs/{job_id}/sprite.jpg", s.getSprite)
	mux.HandleFunc("POST /api/v1/jobs/{job_id}/renders", s.createRender)
	mux.HandleFunc("GET /api/v1/renders/{render_id}", s.getRender)
	mux.HandleFunc("GET /api/v1/renders/{render_id}/sticker.webp", s.getSticker)
	mux.HandleFunc("GET /api/v1/renders/{render_id}/tray.webp", s.getTray)
	return recoverer(cors(accessLog(mux)))
}

// allow enforces the per-device rate limit; it writes the 429 itself.
func (s *Server) allow(w http.ResponseWriter, r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	device := r.Header.Get("X-Device-Id")
	if device == "" {
		device = "no-device"
	}
	if s.Limiter.Allow(device + ":" + ip) {
		return true
	}
	writeError(w, apperr.New(models.RateLimited, "Demasiadas solicitudes. Probá de nuevo en un rato.").
		WithStatus(http.StatusTooManyRequests))
	return false
}

// Command server is the Stickers Maker backend: it turns a TikTok link into
// a WhatsApp sticker. Downloading is delegated to the yt-dlp downloader
// service; everything else (probe, previews, rendering) happens here.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"stickers-server/internal/api"
	"stickers-server/internal/config"
	"stickers-server/internal/download"
	"stickers-server/internal/sticker"
	"stickers-server/internal/storage"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe http://localhost:$PORT/health and exit 0/1 (for Docker)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *healthcheck {
		os.Exit(probeHealth(cfg.Port))
	}
	setupLogging(cfg.LogLevel)

	if err := run(cfg); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Settings) error {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("%s not found in PATH: it is required to render stickers", bin)
		}
	}
	if err := os.MkdirAll(cfg.StorageDir, 0o755); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := storage.New(cfg)
	downloader := download.NewHTTPClient(cfg.DownloaderURL, cfg.DownloadTotalTimeout)
	srv := api.New(ctx, cfg, store, downloader, sticker.NewFitter(cfg))

	cleanupDone := make(chan struct{})
	go store.RunCleanupLoop(cleanupDone)
	defer close(cleanupDone)

	httpSrv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: proxy videos are streamed to slow phones.
	}

	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.ListenAndServe() }()
	slog.Info("stickers-server starting", "addr", httpSrv.Addr, "storage_dir", cfg.StorageDir,
		"downloader", cfg.DownloaderURL)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Warn("http shutdown", "err", err)
	}
	// ctx is already cancelled, so in-flight jobs are winding down and
	// marking themselves failed; wait for that so job.json is consistent.
	srv.Wait()
	return nil
}

func setupLogging(level string) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}

func probeHealth(port int) int {
	c := http.Client{Timeout: 4 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

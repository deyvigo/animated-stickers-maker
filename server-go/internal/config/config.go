// Package config loads settings from environment variables (and an
// optional .env file), with the same defaults as the Python server.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Settings struct {
	StorageDir      string
	JobTTL          time.Duration
	CleanupInterval time.Duration

	MaxConcurrentDownloads int
	MaxConcurrentRenders   int
	MaxSourceFilesizeMB    int

	AllowedHosts []string

	// RateLimitPerHour <= 0 disables rate limiting (single local user).
	RateLimitPerHour int

	// DownloadTimeout is yt-dlp's socket timeout (enforced by the
	// downloader service); DownloadTotalTimeout bounds the whole HTTP call
	// to it from this server.
	DownloadTimeout      time.Duration
	DownloadTotalTimeout time.Duration
	FFmpegTimeout        time.Duration

	// DownloaderURL is the base URL of the yt-dlp downloader service.
	DownloaderURL string

	// WhatsApp sticker hard limits.
	StickerDimension int
	StaticMaxBytes   int64
	AnimatedMaxBytes int64
	TrayDimension    int
	TrayMaxBytes     int64
	MaxClipSeconds   float64

	// PaddingFill is "blur" (opaque blurred-video fill, cheaper in bytes)
	// or "transparent". See sticker.PaddingFill.
	PaddingFill string

	Host     string
	Port     int
	LogLevel string
}

func Default() Settings {
	return Settings{
		StorageDir:             "/data/storage",
		JobTTL:                 6 * time.Hour,
		CleanupInterval:        900 * time.Second,
		MaxConcurrentDownloads: 2,
		MaxConcurrentRenders:   2,
		MaxSourceFilesizeMB:    80,
		AllowedHosts: []string{
			"tiktok.com", "www.tiktok.com", "m.tiktok.com", "vm.tiktok.com", "vt.tiktok.com",
		},
		RateLimitPerHour:     60,
		DownloadTimeout:      120 * time.Second,
		DownloadTotalTimeout: 300 * time.Second,
		FFmpegTimeout:        60 * time.Second,
		DownloaderURL:        "http://downloader:8000",
		StickerDimension:     512,
		StaticMaxBytes:       100 * 1024,
		AnimatedMaxBytes:     500 * 1024,
		TrayDimension:        96,
		TrayMaxBytes:         50 * 1024,
		MaxClipSeconds:       10.0,
		PaddingFill:          "blur",
		Host:                 "0.0.0.0",
		Port:                 8000,
		LogLevel:             "info",
	}
}

func (s Settings) JobDir(jobID string) string {
	return filepath.Join(s.StorageDir, "jobs", jobID)
}

// RenderDir is flat and independent of the job id, so /renders/{id} routes
// don't need the job id in the URL.
func (s Settings) RenderDir(renderID string) string {
	return filepath.Join(s.StorageDir, "renders", renderID)
}

// Load reads ".env" (if present, without overriding real env vars) and then
// applies environment overrides on top of Default().
func Load() (Settings, error) {
	return LoadFrom(".env", os.LookupEnv)
}

// LookupFunc matches os.LookupEnv, so tests can inject their own env.
type LookupFunc func(key string) (string, bool)

func LoadFrom(envFile string, lookup LookupFunc) (Settings, error) {
	fileVars := readDotEnv(envFile)
	get := func(key string) (string, bool) {
		if v, ok := lookup(key); ok {
			return v, true
		}
		v, ok := fileVars[key]
		return v, ok
	}

	s := Default()
	var errs []string

	str := func(key string, dst *string) {
		if v, ok := get(key); ok && v != "" {
			*dst = v
		}
	}
	integer := func(key string, dst *int) {
		if v, ok := get(key); ok && v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", key, err))
				return
			}
			*dst = n
		}
	}
	seconds := func(key string, dst *time.Duration) {
		if v, ok := get(key); ok && v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", key, err))
				return
			}
			*dst = time.Duration(f * float64(time.Second))
		}
	}

	str("STORAGE_DIR", &s.StorageDir)
	str("HOST", &s.Host)
	str("LOG_LEVEL", &s.LogLevel)
	str("DOWNLOADER_URL", &s.DownloaderURL)
	str("PADDING_FILL", &s.PaddingFill)
	integer("PORT", &s.Port)
	integer("MAX_CONCURRENT_DOWNLOADS", &s.MaxConcurrentDownloads)
	integer("MAX_CONCURRENT_RENDERS", &s.MaxConcurrentRenders)
	integer("MAX_SOURCE_FILESIZE_MB", &s.MaxSourceFilesizeMB)
	integer("RATE_LIMIT_PER_HOUR", &s.RateLimitPerHour)
	seconds("CLEANUP_INTERVAL_SECONDS", &s.CleanupInterval)
	seconds("DOWNLOAD_TIMEOUT_SECONDS", &s.DownloadTimeout)
	seconds("DOWNLOAD_TOTAL_TIMEOUT_SECONDS", &s.DownloadTotalTimeout)
	seconds("FFMPEG_TIMEOUT_SECONDS", &s.FFmpegTimeout)

	if v, ok := get("JOB_TTL_HOURS"); ok && v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			errs = append(errs, fmt.Sprintf("JOB_TTL_HOURS: %v", err))
		} else {
			s.JobTTL = time.Duration(f * float64(time.Hour))
		}
	}
	if v, ok := get("ALLOWED_HOSTS"); ok && v != "" {
		s.AllowedHosts = nil
		for _, h := range strings.Split(v, ",") {
			if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
				s.AllowedHosts = append(s.AllowedHosts, h)
			}
		}
	}

	if s.PaddingFill != "blur" && s.PaddingFill != "transparent" {
		errs = append(errs, `PADDING_FILL must be "blur" or "transparent"`)
	}
	if s.MaxConcurrentDownloads < 1 || s.MaxConcurrentRenders < 1 {
		errs = append(errs, "MAX_CONCURRENT_* must be >= 1")
	}
	if len(errs) > 0 {
		return s, fmt.Errorf("invalid configuration: %s", strings.Join(errs, "; "))
	}
	return s, nil
}

// readDotEnv parses simple KEY=VALUE lines; missing file is not an error.
func readDotEnv(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		out[strings.TrimSpace(k)] = v
	}
	return out
}

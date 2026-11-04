package download

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"stickers-server/internal/apperr"
	"stickers-server/internal/models"
)

// Downloader fetches the video for a job into the job's directory (shared
// storage), as `source.<ext>`.
type Downloader interface {
	Download(ctx context.Context, jobID, url string) error
}

// HTTPClient calls the yt-dlp downloader service. Both services mount the
// same storage volume, so only the job id travels over HTTP — the service
// derives the directory itself, which rules out path injection.
type HTTPClient struct {
	base string
	hc   *http.Client
}

func NewHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		base: strings.TrimRight(baseURL, "/"),
		hc:   &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) Download(ctx context.Context, jobID, videoURL string) error {
	body, _ := json.Marshal(map[string]string{"job_id": jobID, "url": videoURL})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/download", bytes.NewReader(body))
	if err != nil {
		return apperr.New(models.DownloadFailed, "Falló la descarga del video.").WithDetail(err.Error())
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var ne interface{ Timeout() bool }
		if errors.As(err, &ne) && ne.Timeout() {
			return apperr.New(models.Timeout, "La descarga del video tardó demasiado.")
		}
		return apperr.New(models.DownloadFailed, "El servicio de descarga no está disponible.").
			WithDetail(err.Error())
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 400 {
		return nil
	}
	var e models.APIError
	if json.Unmarshal(data, &e) == nil && e.Code != "" && e.Message != "" {
		ae := apperr.New(e.Code, e.Message)
		if e.Detail != nil {
			ae = ae.WithDetail(*e.Detail)
		}
		return ae
	}
	return apperr.New(models.DownloadFailed, "Falló la descarga del video.").
		WithDetail(fmt.Sprintf("downloader returned HTTP %d", resp.StatusCode))
}

package media

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"stickers-server/internal/apperr"
	"stickers-server/internal/models"
)

const defaultProbeTimeout = 30 * time.Second

type ffprobeOutput struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
		RFrameRate   string `json:"r_frame_rate"`
		Duration     string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func ffprobeJSON(ctx context.Context, path string, timeout time.Duration) (*ffprobeOutput, error) {
	stdout, stderr, timedOut, err := run(ctx, timeout, "ffprobe",
		"-v", "error", "-print_format", "json", "-show_format", "-show_streams", path)
	if timedOut {
		return nil, apperr.New(models.Timeout, "El análisis del video tardó demasiado.")
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		detail := head(stderr, 500)
		if detail == "" {
			detail = err.Error()
		}
		return nil, apperr.New(models.ExtractorError, "No se pudo leer el video descargado.").WithDetail(detail)
	}
	var out ffprobeOutput
	if err := json.Unmarshal(stdout, &out); err != nil {
		return nil, apperr.New(models.ExtractorError, "No se pudo leer el video descargado.").WithDetail(err.Error())
	}
	return &out, nil
}

// ParseFPS parses ffprobe frame rates like "30000/1001" or "25". Invalid
// input and zero denominators yield 0.
func ParseFPS(rate string) float64 {
	if num, den, ok := strings.Cut(rate, "/"); ok {
		n, err1 := strconv.ParseFloat(num, 64)
		d, err2 := strconv.ParseFloat(den, 64)
		if err1 != nil || err2 != nil || d == 0 {
			return 0
		}
		return n / d
	}
	f, err := strconv.ParseFloat(rate, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseSeconds(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// ProbeVideo extracts duration, size and fps of the first video stream.
func ProbeVideo(ctx context.Context, path string) (models.JobMetadata, error) {
	data, err := ffprobeJSON(ctx, path, defaultProbeTimeout)
	if err != nil {
		return models.JobMetadata{}, err
	}
	for _, s := range data.Streams {
		if s.CodecType != "video" {
			continue
		}
		if s.Width <= 0 || s.Height <= 0 {
			break
		}
		duration := parseSeconds(data.Format.Duration)
		if duration == 0 {
			duration = parseSeconds(s.Duration)
		}
		// avg_frame_rate can be "0/0" for some containers; r_frame_rate
		// is then the best remaining estimate.
		fps := ParseFPS(s.AvgFrameRate)
		if fps == 0 {
			fps = ParseFPS(s.RFrameRate)
		}
		return models.JobMetadata{Duration: duration, Width: s.Width, Height: s.Height, FPS: fps}, nil
	}
	return models.JobMetadata{}, apperr.New(models.ExtractorError, "El archivo descargado no tiene una pista de video.")
}

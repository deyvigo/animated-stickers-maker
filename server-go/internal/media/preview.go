package media

import (
	"context"
	"fmt"
	"math"
	"time"

	"stickers-server/internal/apperr"
	"stickers-server/internal/models"
)

const (
	proxyHeight       = 480
	spriteColumns     = 8
	spriteTileWidth   = 120
	defaultSpriteTile = 32
)

var previewMsgs = Msgs{
	Timeout: "La generación de la vista previa tardó demasiado.",
	Failed:  "No se pudo generar la vista previa del video.",
}

// GenerateProxy writes a small H.264 proxy for smooth scrubbing. It forces
// a keyframe every 0.5s: the editor preview seeks back to a segment's start
// on every loop (and between segments), and with x264's default GOP (up to
// ~8s) each seek had to decode from a keyframe seconds earlier, which
// showed up as a visible stall.
func GenerateProxy(ctx context.Context, source, out string, timeout time.Duration) error {
	return RunFFmpeg(ctx, timeout, previewMsgs,
		"-i", source,
		"-vf", fmt.Sprintf("scale=-2:%d", proxyHeight),
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "28",
		"-force_key_frames", "expr:gte(t,n_forced*0.5)",
		"-c:a", "aac", "-b:a", "96k",
		"-movflags", "+faststart",
		out,
	)
}

// SpriteArgs builds the ffmpeg arguments for the thumbnail grid. Split out
// so the filter string can be unit-tested without running ffmpeg.
func SpriteArgs(source, out string, duration float64, frameCount int) ([]string, error) {
	if duration <= 0 {
		return nil, apperr.New(models.ExtractorError, "El video no tiene una duración válida.")
	}
	fps := float64(frameCount) / duration
	rows := int(math.Ceil(float64(frameCount) / spriteColumns))
	return []string{
		"-i", source,
		"-vf", fmt.Sprintf("fps=%.6f,scale=%d:-1,tile=%dx%d", fps, spriteTileWidth, spriteColumns, rows),
		"-frames:v", "1",
		"-q:v", "4",
		out,
	}, nil
}

// GenerateSprite writes one JPEG grid of evenly spaced thumbnails, used as
// the timeline background so scrubbing doesn't re-decode the source.
func GenerateSprite(ctx context.Context, source, out string, duration float64, timeout time.Duration) error {
	args, err := SpriteArgs(source, out, duration, defaultSpriteTile)
	if err != nil {
		return err
	}
	return RunFFmpeg(ctx, timeout, previewMsgs, args...)
}

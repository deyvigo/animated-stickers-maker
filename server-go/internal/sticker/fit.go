package sticker

import (
	"context"
	"fmt"
	"os"
	"strings"

	"stickers-server/internal/apperr"
	"stickers-server/internal/config"
	"stickers-server/internal/media"
	"stickers-server/internal/models"
	"stickers-server/internal/webpinfo"
)

const (
	// If even the bottom of the fps/quality search doesn't fit, shrink the
	// clip by this factor and search again, down to a floor duration.
	durationShrinkFactor = 0.7
	minClipSeconds       = 0.6
	maxDurationRetries   = 4

	// Per-frame durations in WebP are integer milliseconds, so the sum of
	// frames can land a few ms past the requested length. WhatsApp rejects
	// anything over its cap, so encode slightly under it.
	durationSafetyMargin = 0.05

	// Quality thresholds for the "calidad reducida" note in the app.
	degradedAnimatedFPS = 15
	degradedStillQ      = 70

	stillQHigh = 90
	stillQLow  = 20
)

var (
	renderMsgs = media.Msgs{
		Timeout: "La generación del sticker tardó demasiado.",
		Failed:  "No se pudo generar el sticker.",
	}
)

// Fitter renders stickers that fit WhatsApp's size budgets.
type Fitter struct {
	S      config.Settings
	Policy Policy
}

func NewFitter(s config.Settings) *Fitter {
	return &Fitter{S: s, Policy: DefaultPolicy}
}

func sibling(out, tag string) string {
	return strings.TrimSuffix(out, ".webp") + "." + tag + ".webp"
}

// bestFile keeps the best fitting encode on disk while the search runs: each
// attempt is written to a scratch path and promoted when it improves on the
// current best, so no final re-encode is needed.
type bestFile struct {
	try, best string
	has       bool
}

func newBestFile(out string) *bestFile {
	b := &bestFile{try: sibling(out, "try"), best: sibling(out, "best")}
	b.cleanup()
	return b
}

func (b *bestFile) promote() error {
	b.has = true
	return os.Rename(b.try, b.best)
}

func (b *bestFile) cleanup() {
	os.Remove(b.try)
	os.Remove(b.best)
	b.has = false
}

func (b *bestFile) finish(out string) error {
	if err := os.Rename(b.best, out); err != nil {
		return err
	}
	b.has = false
	os.Remove(b.try)
	return nil
}

func fileSize(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

func (f *Fitter) animatedArgs(source, out string, start, duration float64, vf string, q int) []string {
	return []string{
		"-ss", fmt.Sprintf("%.3f", start),
		"-t", fmt.Sprintf("%.3f", duration),
		"-i", source,
		"-vf", vf,
		"-an",
		"-loop", "0",
		"-c:v", "libwebp_anim",
		"-lossless", "0",
		"-q:v", fmt.Sprint(q),
		// compression_level trades encode speed for size, not visual
		// quality. Measured on 8s clips: level 6 is ~10x slower than 4
		// for ~3% smaller files, which isn't worth it per ladder rung.
		"-compression_level", "4",
		"-fps_mode", "passthrough",
		out,
	}
}

// FitAnimated renders [start, end) of source as an animated sticker under
// the animated byte budget, writing the result to out.
func (f *Fitter) FitAnimated(ctx context.Context, source, out string, start, end float64,
	crop CropPixels, srcW, srcH int) (models.RenderFitInfo, error) {

	duration := min(end-start, f.S.MaxClipSeconds-durationSafetyMargin)
	bf := newBestFile(out)
	defer bf.cleanup()

	for attempt := 0; attempt <= maxDurationRetries; attempt++ {
		var bestAttempt Attempt
		enc := func(ctx context.Context, a Attempt) (int64, error) {
			vf := CropFilter(crop, srcW, srcH, f.S.StickerDimension, a.FPS)
			if err := media.RunFFmpeg(ctx, f.S.FFmpegTimeout, renderMsgs,
				f.animatedArgs(source, bf.try, start, duration, vf, a.Quality)...); err != nil {
				return 0, err
			}
			size, err := fileSize(bf.try)
			if err != nil {
				return 0, err
			}
			if size <= f.S.AnimatedMaxBytes && (!bf.has || better(a, bestAttempt)) {
				if err := bf.promote(); err != nil {
					return 0, err
				}
				bestAttempt = a
			}
			return size, nil
		}

		best, err := Search(ctx, f.Policy, f.S.AnimatedMaxBytes, enc)
		if err != nil {
			return models.RenderFitInfo{}, err
		}
		if best != nil {
			if err := bf.finish(out); err != nil {
				return models.RenderFitInfo{}, err
			}
			if err := f.validate(out, true); err != nil {
				return models.RenderFitInfo{}, err
			}
			fps, q, dur := float64(best.FPS), best.Quality, duration
			info := models.RenderFitInfo{
				FPS:      &fps,
				Quality:  &q,
				Duration: &dur,
				Degraded: best.FPS < degradedAnimatedFPS || best.Quality < f.Policy.QFloor || attempt > 0,
			}
			if attempt > 0 {
				note := fmt.Sprintf("duración recortada a %.1fs", duration)
				info.Note = &note
			}
			return info, nil
		}
		bf.cleanup()
		// Bottom of the search still too big: shrink duration and retry.
		duration = max(minClipSeconds, duration*durationShrinkFactor)
	}

	return models.RenderFitInfo{}, apperr.New(models.RenderFailedCode,
		"No se pudo comprimir el sticker animado dentro del límite de WhatsApp (500 KB). "+
			"Probá con un recorte más corto o una zona con menos detalle.")
}

func stillArgs(source, out string, frameAt float64, vf string, q int) []string {
	return []string{
		"-ss", fmt.Sprintf("%.3f", frameAt),
		"-i", source,
		"-vf", vf,
		"-frames:v", "1",
		"-c:v", "libwebp",
		"-compression_level", "6",
		"-q:v", fmt.Sprint(q),
		out,
	}
}

// fitStill encodes one frame at the highest quality that fits limit.
// Returns the chosen quality, or -1 if even stillQLow is too big.
func (f *Fitter) fitStill(ctx context.Context, source, out string, frameAt float64,
	vf string, limit int64) (int, error) {

	bf := newBestFile(out)
	defer bf.cleanup()
	bestQ := -1
	q, err := SearchQuality(ctx, stillQLow, stillQHigh, limit, func(ctx context.Context, q int) (int64, error) {
		if err := media.RunFFmpeg(ctx, f.S.FFmpegTimeout, renderMsgs, stillArgs(source, bf.try, frameAt, vf, q)...); err != nil {
			return 0, err
		}
		size, err := fileSize(bf.try)
		if err != nil {
			return 0, err
		}
		if size <= limit && q > bestQ {
			if err := bf.promote(); err != nil {
				return 0, err
			}
			bestQ = q
		}
		return size, nil
	})
	if err != nil || q < 0 {
		return -1, err
	}
	return q, bf.finish(out)
}

// FitStatic renders a single frame as a static sticker under the static budget.
func (f *Fitter) FitStatic(ctx context.Context, source, out string, frameAt float64,
	crop CropPixels, srcW, srcH int) (models.RenderFitInfo, error) {

	vf := CropFilter(crop, srcW, srcH, f.S.StickerDimension, 0)
	q, err := f.fitStill(ctx, source, out, frameAt, vf, f.S.StaticMaxBytes)
	if err != nil {
		return models.RenderFitInfo{}, err
	}
	if q < 0 {
		return models.RenderFitInfo{}, apperr.New(models.RenderFailedCode,
			"No se pudo comprimir el sticker dentro del límite de WhatsApp (100 KB).")
	}
	if err := f.validate(out, false); err != nil {
		return models.RenderFitInfo{}, err
	}
	return models.RenderFitInfo{Quality: &q, Degraded: q < degradedStillQ}, nil
}

// RenderTray writes the 96x96 tray icon derived from the same frame/crop.
// Always WebP (WhatsApp accepts PNG or WebP for the tray icon).
func (f *Fitter) RenderTray(ctx context.Context, source, out string, frameAt float64,
	crop CropPixels, srcW, srcH int) error {

	vf := CropFilter(crop, srcW, srcH, f.S.TrayDimension, 0)
	q, err := f.fitStill(ctx, source, out, frameAt, vf, f.S.TrayMaxBytes)
	if err != nil {
		return err
	}
	if q < 0 {
		return apperr.New(models.RenderFailedCode, "No se pudo generar el ícono del pack.")
	}
	return nil
}

// validate never hands back a file WhatsApp would reject: it verifies
// dimensions and duration by parsing the WebP itself rather than trusting
// the ffmpeg filter graph.
func (f *Fitter) validate(path string, animated bool) error {
	info, err := webpinfo.ReadFile(path)
	if err != nil {
		return apperr.New(models.RenderFailedCode, "El sticker generado quedó inválido.").WithDetail(err.Error())
	}
	d := f.S.StickerDimension
	if info.Width != d || info.Height != d {
		return apperr.New(models.RenderFailedCode, fmt.Sprintf("El sticker generado no quedó en %dx%d.", d, d)).
			WithDetail(fmt.Sprintf("%+v", info))
	}
	// WhatsApp's limit is "duration must not exceed 10000ms": strictly
	// greater is invalid, so a render landing exactly on the cap passes.
	if animated && info.Duration > f.S.MaxClipSeconds {
		return apperr.New(models.RenderFailedCode, "El sticker animado quedó más largo de lo permitido.").
			WithDetail(fmt.Sprintf("%+v", info))
	}
	return nil
}

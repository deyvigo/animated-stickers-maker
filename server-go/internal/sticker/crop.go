// Package sticker renders a crop/time range of a source video into a
// WhatsApp-compliant sticker: exactly 512x512 WebP, animated (<500KB) or
// static (<100KB), plus the 96x96 tray icon.
package sticker

import (
	"fmt"
	"math"
	"strings"

	"stickers-server/internal/models"
)

// CropPixels may be negative, and X+Width / Y+Height may exceed the source
// frame: the editor's zoom/pan UI can legitimately ask for a crop bigger
// than, or offset outside, the actual video, expecting the uncovered area
// to come back as padding.
type CropPixels struct {
	X, Y, Width, Height int
}

// even rounds toward negative infinity to an even integer. The int
// conversion truncates toward zero (like Python's int()), then clearing
// the low bit floors to even for negatives too: -3 -> -4, 3 -> 2.
func even(n float64) int { return int(n) &^ 1 }

func evenSize(n float64) int { return max(2, even(n)) }

// CropToPixels converts a normalized crop rect to even pixel coordinates.
// It deliberately does NOT clamp to the source frame. Even dimensions avoid
// odd-size chroma subsampling issues in the scale/pad filter chain.
func CropToPixels(c models.CropRect, srcW, srcH int) CropPixels {
	return CropPixels{
		X:      even(c.X * float64(srcW)),
		Y:      even(c.Y * float64(srcH)),
		Width:  evenSize(c.Width * float64(srcW)),
		Height: evenSize(c.Height * float64(srcH)),
	}
}

// PaddingFill says what goes in the areas of the square canvas that the
// video doesn't cover (letterbox bars, or a crop reaching outside the frame).
type PaddingFill string

const (
	// FillBlur fills them with a heavily blurred copy of the video itself
	// (the TikTok look). It is opaque, which matters: real transparency
	// makes libwebp spend roughly a third more bytes on the same frame,
	// costing several fps inside the 500KB budget.
	FillBlur PaddingFill = "blur"
	// FillTransparent leaves them transparent: a clean cutout on the chat
	// background, at the price of fewer fps.
	FillTransparent PaddingFill = "transparent"
)

// FilterOpts parameterizes CropFilter.
type FilterOpts struct {
	FPS  int         // 0 = keep the source frame rate (stills)
	Blur float64     // gaussian sigma applied to the final frame, 0 = none
	Fill PaddingFill // "" is treated as FillTransparent
}

// CropFilter builds the ffmpeg -vf graph that turns the crop into a
// dimension x dimension frame.
func CropFilter(c CropPixels, srcW, srcH, dimension int, o FilterOpts) string {
	var vf string
	if o.Fill == FillBlur {
		vf = blurFillFilter(c, srcW, srcH, dimension, o.FPS)
	}
	if vf == "" {
		vf = transparentFilter(c, srcW, srcH, dimension, o.FPS)
	}
	if o.Blur > 0 {
		vf += fmt.Sprintf(",gblur=sigma=%.2f", o.Blur)
	}
	return vf
}

// transparentFilter: optional pad (when the crop leaves the frame), crop,
// optional fps, then scale+pad to a square, padding with transparency.
func transparentFilter(c CropPixels, srcW, srcH, dimension, fps int) string {
	// ffmpeg's crop can't read pixels that don't exist, so when the crop
	// leaves the frame we first pad the source out to a big-enough
	// transparent canvas and shift the crop origin to match. When the crop
	// is fully inside the frame (the common case) all margins are 0 and
	// this is skipped.
	left := max(0, -c.X)
	top := max(0, -c.Y)
	right := max(0, c.X+c.Width-srcW)
	bottom := max(0, c.Y+c.Height-srcH)

	var parts []string
	cropX, cropY := c.X, c.Y
	if left != 0 || top != 0 || right != 0 || bottom != 0 {
		// format=yuva420p forces an alpha plane onto the (opaque) decoded
		// source before pad runs; without it pad's transparent color is
		// silently discarded and the area comes back opaque black.
		parts = append(parts, "format=yuva420p")
		parts = append(parts, fmt.Sprintf("pad=%d:%d:%d:%d:color=#00000000",
			srcW+left+right, srcH+top+bottom, left, top))
		cropX, cropY = c.X+left, c.Y+top
	}
	parts = append(parts, fmt.Sprintf("crop=%d:%d:%d:%d", c.Width, c.Height, cropX, cropY))
	if fps > 0 {
		parts = append(parts, fmt.Sprintf("fps=%d", fps))
	}
	parts = append(parts,
		fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease:flags=lanczos", dimension, dimension),
		fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=#00000000", dimension, dimension),
	)
	return strings.Join(parts, ",")
}

// blurFillFilter composes the visible part of the crop over a blurred,
// darkened, cover-scaled copy of itself. It returns "" when no fill is
// needed (the video already covers the whole canvas) or when the crop
// doesn't touch the frame at all; the caller then uses the plain chain.
func blurFillFilter(c CropPixels, srcW, srcH, dim, fps int) string {
	// The canvas shows the crop scaled by s, centered (same geometry as
	// the transparent chain's scale + pad).
	s := math.Min(float64(dim)/float64(c.Width), float64(dim)/float64(c.Height))

	// Only the part of the crop inside the source frame has pixels.
	ix0, iy0 := max(c.X, 0), max(c.Y, 0)
	ix1, iy1 := min(c.X+c.Width, srcW), min(c.Y+c.Height, srcH)
	iw, ih := ix1-ix0, iy1-iy0
	if iw <= 0 || ih <= 0 {
		return ""
	}

	fw := min(dim, max(2, int(math.Round(float64(iw)*s))))
	fh := min(dim, max(2, int(math.Round(float64(ih)*s))))
	// Even offsets keep yuv420 chroma aligned in overlay.
	x := int(math.Round((float64(dim)-float64(c.Width)*s)/2+float64(ix0-c.X)*s)) &^ 1
	y := int(math.Round((float64(dim)-float64(c.Height)*s)/2+float64(iy0-c.Y)*s)) &^ 1

	// Foreground already fills the canvas (±1px of rounding): no bars.
	if x <= 1 && y <= 1 && x+fw >= dim-1 && y+fh >= dim-1 {
		return ""
	}

	// Scale the blur with the canvas so the 96px tray looks like the sticker.
	sigma := 24.0 * float64(dim) / 512.0

	parts := []string{fmt.Sprintf("crop=%d:%d:%d:%d", iw, ih, ix0, iy0)}
	if fps > 0 {
		parts = append(parts, fmt.Sprintf("fps=%d", fps))
	}
	graph := strings.Join(parts, ",") +
		fmt.Sprintf(",split[a][b];"+
			"[a]scale=%d:%d:force_original_aspect_ratio=increase:flags=bilinear,crop=%d:%d,"+
			"gblur=sigma=%.2f,eq=brightness=-0.12[bg];"+
			"[b]scale=%d:%d:flags=lanczos[fg];"+
			"[bg][fg]overlay=%d:%d,format=yuv420p",
			dim, dim, dim, dim, sigma, fw, fh, x, y)
	return graph
}

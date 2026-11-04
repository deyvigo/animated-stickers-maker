// Package sticker renders a crop/time range of a source video into a
// WhatsApp-compliant sticker: exactly 512x512 WebP, animated (<500KB) or
// static (<100KB), plus the 96x96 tray icon.
package sticker

import (
	"fmt"
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

// CropFilter builds the ffmpeg -vf chain: optional pad (when the crop
// leaves the frame), crop, optional fps, then scale+pad to a square.
func CropFilter(c CropPixels, srcW, srcH, dimension int, fps int) string {
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

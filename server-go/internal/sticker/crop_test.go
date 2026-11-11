package sticker

import (
	"strings"
	"testing"

	"stickers-server/internal/models"
)

func TestEvenMatchesPythonSemantics(t *testing.T) {
	// Python: i = int(n); i - (i % 2)  — floors to even, also for negatives.
	cases := map[float64]int{0: 0, 1: 0, 2: 2, 3.9: 2, -1: -2, -3: -4, -4: -4, -3.9: -4 + 0}
	cases[-3.9] = -4 // int(-3.9) = -3 -> -4
	for in, want := range cases {
		if got := even(in); got != want {
			t.Errorf("even(%v) = %d, want %d", in, got, want)
		}
	}
	if evenSize(0.5) != 2 || evenSize(5) != 4 {
		t.Error("evenSize must floor to even with a minimum of 2")
	}
}

func TestCropToPixelsDoesNotClamp(t *testing.T) {
	got := CropToPixels(models.CropRect{X: -0.25, Y: 0, Width: 1.5, Height: 1}, 720, 1280)
	want := CropPixels{X: -180, Y: 0, Width: 1080, Height: 1280}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestCropFilterInsideFrameHasNoPad(t *testing.T) {
	vf := CropFilter(CropPixels{X: 0, Y: 280, Width: 720, Height: 720}, 720, 1280, 512, FilterOpts{FPS: 12})
	want := "crop=720:720:0:280,fps=12,scale=512:512:force_original_aspect_ratio=decrease:flags=lanczos," +
		"pad=512:512:(ow-iw)/2:(oh-ih)/2:color=#00000000"
	if vf != want {
		t.Fatalf("\n got %s\nwant %s", vf, want)
	}
}

func TestCropFilterOutsideFramePadsFirst(t *testing.T) {
	vf := CropFilter(CropPixels{X: -100, Y: 0, Width: 920, Height: 1280}, 720, 1280, 512, FilterOpts{})
	if !strings.HasPrefix(vf, "format=yuva420p,pad=920:1280:100:0:color=#00000000,crop=920:1280:0:0,") {
		t.Fatalf("unexpected prefix: %s", vf)
	}
	if strings.Contains(vf, "fps=") {
		t.Error("fps must be omitted when 0")
	}
}

func TestBlurFillForLetterbox(t *testing.T) {
	// Whole 720x1280 video letterboxed into the square: crop 1280x1280
	// starting 280px left of the frame (the editor's default).
	c := CropPixels{X: -280, Y: 0, Width: 1280, Height: 1280}
	vf := CropFilter(c, 720, 1280, 512, FilterOpts{FPS: 10, Fill: FillBlur})

	for _, want := range []string{
		"crop=720:1280:0:0,fps=10,split[a][b];",              // only the in-frame part is cropped
		"[b]scale=288:512:flags=lanczos[fg]",                 // 0.4x, aspect kept
		"[bg][fg]overlay=112:0,format=yuv420p",               // centered: (512-288)/2
		"scale=512:512:force_original_aspect_ratio=increase", // bg covers the canvas
	} {
		if !strings.Contains(vf, want) {
			t.Errorf("missing %q in:\n%s", want, vf)
		}
	}
	if strings.Contains(vf, "yuva420p") || strings.Contains(vf, "#00000000") {
		t.Errorf("blur fill must stay opaque:\n%s", vf)
	}
}

func TestBlurFillNotUsedWhenVideoCoversCanvas(t *testing.T) {
	inside := CropPixels{X: 0, Y: 280, Width: 720, Height: 720}
	plain := CropFilter(inside, 720, 1280, 512, FilterOpts{FPS: 12})
	if got := CropFilter(inside, 720, 1280, 512, FilterOpts{FPS: 12, Fill: FillBlur}); got != plain {
		t.Errorf("no bars -> same chain as transparent mode:\n got %s\nwant %s", got, plain)
	}
}

func TestBlurFillCropOutsideFrameKeepsOffsets(t *testing.T) {
	// Crop 100px past the right edge of a 400x400 frame: the video covers
	// the left 3/5 of the canvas and the rest is fill.
	c := CropPixels{X: 0, Y: 0, Width: 500, Height: 400}
	vf := CropFilter(c, 400, 400, 500, FilterOpts{Fill: FillBlur})
	if !strings.Contains(vf, "crop=400:400:0:0,split") || !strings.Contains(vf, "[b]scale=400:400") ||
		!strings.Contains(vf, "overlay=0:50") {
		t.Errorf("unexpected graph:\n%s", vf)
	}
}

func TestBlurFillFallsBackWhenCropMissesTheFrame(t *testing.T) {
	c := CropPixels{X: 2000, Y: 2000, Width: 100, Height: 100}
	vf := CropFilter(c, 720, 1280, 512, FilterOpts{Fill: FillBlur})
	if strings.Contains(vf, "split") {
		t.Errorf("a crop entirely outside the frame has nothing to blur:\n%s", vf)
	}
}

func TestFillSigmaScalesWithCanvas(t *testing.T) {
	c := CropPixels{X: -280, Y: 0, Width: 1280, Height: 1280}
	big := CropFilter(c, 720, 1280, 512, FilterOpts{Fill: FillBlur})
	tray := CropFilter(c, 720, 1280, 96, FilterOpts{Fill: FillBlur})
	if !strings.Contains(big, "gblur=sigma=24.00") || !strings.Contains(tray, "gblur=sigma=4.50") {
		t.Errorf("sigma not scaled:\n%s\n%s", big, tray)
	}
}

func TestPreBlurIsAppendedLast(t *testing.T) {
	vf := CropFilter(CropPixels{Width: 720, Height: 720}, 720, 720, 512, FilterOpts{Blur: 0.6})
	if !strings.HasSuffix(vf, ",gblur=sigma=0.60") {
		t.Errorf("got %s", vf)
	}
}

func TestCropFilterSpeedPrependsSetpts(t *testing.T) {
	c := CropPixels{X: 0, Y: 0, Width: 720, Height: 720}
	for _, fill := range []PaddingFill{FillTransparent, FillBlur} {
		vf := CropFilter(c, 720, 1280, 512, FilterOpts{FPS: 15, Fill: fill, Speed: 1.5})
		if !strings.HasPrefix(vf, "setpts=PTS/1.5000,") {
			t.Errorf("%s: speed not first in chain: %s", fill, vf)
		}
	}
	if vf := CropFilter(c, 720, 1280, 512, FilterOpts{Speed: 1}); strings.Contains(vf, "setpts") {
		t.Errorf("1x must not add setpts: %s", vf)
	}
}

func TestClipSegments(t *testing.T) {
	segs := []models.Segment{{Start: 1, End: 3}, {Start: 5, End: 8}}
	if got := segmentsTotal(segs); got != 5 {
		t.Fatalf("total = %v", got)
	}
	got := clipSegments(segs, 3)
	if len(got) != 2 || got[1] != (models.Segment{Start: 5, End: 6}) {
		t.Errorf("clip to 3 = %+v", got)
	}
	if got := clipSegments(segs, 2); len(got) != 1 || got[0] != segs[0] {
		t.Errorf("clip to 2 = %+v", got)
	}
	if got := clipSegments(segs, 99); len(got) != 2 || got[1] != segs[1] {
		t.Errorf("no clip = %+v", got)
	}
}

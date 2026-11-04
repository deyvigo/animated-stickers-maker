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
	vf := CropFilter(CropPixels{X: 0, Y: 280, Width: 720, Height: 720}, 720, 1280, 512, 12)
	want := "crop=720:720:0:280,fps=12,scale=512:512:force_original_aspect_ratio=decrease:flags=lanczos," +
		"pad=512:512:(ow-iw)/2:(oh-ih)/2:color=#00000000"
	if vf != want {
		t.Fatalf("\n got %s\nwant %s", vf, want)
	}
}

func TestCropFilterOutsideFramePadsFirst(t *testing.T) {
	vf := CropFilter(CropPixels{X: -100, Y: 0, Width: 920, Height: 1280}, 720, 1280, 512, 0)
	if !strings.HasPrefix(vf, "format=yuva420p,pad=920:1280:100:0:color=#00000000,crop=920:1280:0:0,") {
		t.Fatalf("unexpected prefix: %s", vf)
	}
	if strings.Contains(vf, "fps=") {
		t.Error("fps must be omitted when 0")
	}
}

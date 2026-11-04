package sticker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"stickers-server/internal/config"
	"stickers-server/internal/media"
	"stickers-server/internal/models"
	"stickers-server/internal/webpinfo"
)

func requireLibwebp(t *testing.T) {
	t.Helper()
	out, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "libwebp_anim") {
		t.Skip("ffmpeg with libwebp_anim not available")
	}
}

func testVideo(t *testing.T, seconds int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.mp4")
	err := media.RunFFmpeg(context.Background(), time.Minute, media.Msgs{Timeout: "t", Failed: "f"},
		"-f", "lavfi", "-i", "testsrc2=size=720x1280:rate=30", "-t", itoa(seconds),
		"-vf", "gblur=sigma=1.5,noise=alls=3:allf=t",
		"-pix_fmt", "yuv420p", "-c:v", "libx264", "-crf", "20", path)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func itoa(n int) string { return strconv.Itoa(n) }

func modelsCrop(x, y, w, h float64) models.CropRect {
	return models.CropRect{X: x, Y: y, Width: w, Height: h}
}

func newTestFitter(t *testing.T) *Fitter {
	s := config.Default()
	s.StorageDir = t.TempDir()
	s.FFmpegTimeout = 2 * time.Minute
	return NewFitter(s)
}

func TestFitAnimatedEndToEnd(t *testing.T) {
	requireLibwebp(t)
	src := testVideo(t, 10)
	f := newTestFitter(t)
	crop := CropToPixels(modelsCrop(0, 0.2188, 1, 0.5625), 720, 1280) // 720x720 window

	for _, dur := range []float64{3, 10} {
		out := filepath.Join(t.TempDir(), "sticker.webp")
		info, err := f.FitAnimated(context.Background(), src, out, 0, dur, crop, 720, 1280)
		if err != nil {
			t.Fatalf("dur %v: %v", dur, err)
		}
		st, err := os.Stat(out)
		if err != nil {
			t.Fatal(err)
		}
		if st.Size() > f.S.AnimatedMaxBytes {
			t.Errorf("dur %v: %d bytes over budget", dur, st.Size())
		}
		w, err := webpinfo.ReadFile(out)
		if err != nil || w.Width != 512 || w.Height != 512 || w.Duration > 10 || w.Frames < 2 {
			t.Errorf("dur %v: webp = %+v err=%v", dur, w, err)
		}
		if info.Quality == nil || *info.Quality < f.Policy.QFloor {
			t.Errorf("dur %v: fit = %+v", dur, info)
		}
		t.Logf("dur %v: %.0fKB fps=%v q=%d frames=%d len=%.2fs degraded=%v",
			dur, float64(st.Size())/1024, *info.FPS, *info.Quality, w.Frames, w.Duration, info.Degraded)

		// scratch files must not be left behind
		left, _ := filepath.Glob(filepath.Join(filepath.Dir(out), "*.webp"))
		if len(left) != 1 {
			t.Errorf("leftover files: %v", left)
		}
	}
}

func TestFitStaticTrayAndOutOfFrameCrop(t *testing.T) {
	requireLibwebp(t)
	src := testVideo(t, 2)
	f := newTestFitter(t)
	dir := t.TempDir()

	// Crop wider than the frame, offset to the left: exercises pad+crop.
	crop := CropToPixels(modelsCrop(-0.25, 0, 1.5, 1), 720, 1280)
	stick := filepath.Join(dir, "sticker.webp")
	info, err := f.FitStatic(context.Background(), src, stick, 1, crop, 720, 1280)
	if err != nil {
		t.Fatal(err)
	}
	tray := filepath.Join(dir, "tray.webp")
	if err := f.RenderTray(context.Background(), src, tray, 1, crop, 720, 1280); err != nil {
		t.Fatal(err)
	}
	for path, limit := range map[string]int64{stick: f.S.StaticMaxBytes, tray: f.S.TrayMaxBytes} {
		st, err := os.Stat(path)
		if err != nil || st.Size() > limit {
			t.Errorf("%s: %v size=%v limit=%d", path, err, st, limit)
		}
	}
	w, _ := webpinfo.ReadFile(stick)
	tw, _ := webpinfo.ReadFile(tray)
	if w.Width != 512 || tw.Width != 96 || tw.Height != 96 {
		t.Errorf("sizes: sticker %+v tray %+v", w, tw)
	}
	t.Logf("static q=%d", *info.Quality)
}

package media

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stickers-server/internal/apperr"
	"stickers-server/internal/models"
)

func TestParseFPS(t *testing.T) {
	cases := map[string]float64{
		"30/1": 30, "25": 25, "0/0": 0, "": 0, "x/y": 0, "15/2": 7.5,
	}
	for in, want := range cases {
		if got := ParseFPS(in); got != want {
			t.Errorf("ParseFPS(%q) = %v, want %v", in, got, want)
		}
	}
	if got := ParseFPS("30000/1001"); got < 29.96 || got > 29.98 {
		t.Errorf("ntsc fps = %v", got)
	}
}

func TestSpriteArgs(t *testing.T) {
	args, err := SpriteArgs("in.mp4", "out.jpg", 8, 32)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "fps=4.000000,scale=120:-1,tile=8x4") {
		t.Errorf("unexpected filter: %s", joined)
	}
	if _, err := SpriteArgs("in.mp4", "out.jpg", 0, 32); err == nil {
		t.Error("zero duration should fail")
	}
}

func requireFFmpeg(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
}

// makeVideo renders a 2s 320x240 25fps test clip.
func makeVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.mp4")
	err := RunFFmpeg(context.Background(), 30*time.Second, Msgs{"timeout", "failed"},
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=25",
		"-pix_fmt", "yuv420p", path)
	if err != nil {
		t.Fatalf("make video: %v", err)
	}
	return path
}

func TestProbeVideo(t *testing.T) {
	requireFFmpeg(t)
	md, err := ProbeVideo(context.Background(), makeVideo(t))
	if err != nil {
		t.Fatal(err)
	}
	if md.Width != 320 || md.Height != 240 || md.FPS != 25 || md.Duration < 1.9 || md.Duration > 2.1 {
		t.Fatalf("got %+v", md)
	}
}

func TestProbeVideoRejectsGarbage(t *testing.T) {
	requireFFmpeg(t)
	bad := filepath.Join(t.TempDir(), "bad.mp4")
	os.WriteFile(bad, []byte("definitely not a video"), 0o644)
	_, err := ProbeVideo(context.Background(), bad)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != models.ExtractorError {
		t.Fatalf("err = %v, want extractor_error", err)
	}
}

func TestPreviewAssets(t *testing.T) {
	requireFFmpeg(t)
	src := makeVideo(t)
	dir := t.TempDir()
	proxy, sprite := filepath.Join(dir, "proxy.mp4"), filepath.Join(dir, "sprite.jpg")

	if err := GenerateProxy(context.Background(), src, proxy, time.Minute); err != nil {
		t.Fatal(err)
	}
	md, err := ProbeVideo(context.Background(), proxy)
	if err != nil || md.Height != 480 {
		t.Fatalf("proxy probe: %+v, %v", md, err)
	}

	if err := GenerateSprite(context.Background(), src, sprite, 2, time.Minute); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(sprite); err != nil || st.Size() == 0 {
		t.Fatalf("sprite missing/empty: %v", err)
	}
}

func TestRunFFmpegTimeoutAndFailure(t *testing.T) {
	requireFFmpeg(t)
	msgs := Msgs{Timeout: "too slow", Failed: "broke"}

	// An endless source with a tiny timeout must come back as a timeout.
	err := RunFFmpeg(context.Background(), 200*time.Millisecond, msgs,
		"-f", "lavfi", "-i", "testsrc=size=1920x1080:rate=60", "-f", "null", "-")
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != models.Timeout || ae.Message != "too slow" {
		t.Fatalf("timeout err = %v", err)
	}

	err = RunFFmpeg(context.Background(), 10*time.Second, msgs, "-i", "/nonexistent.mp4", "out.mp4")
	if !errors.As(err, &ae) || ae.Code != models.RenderFailedCode || ae.Detail == "" {
		t.Fatalf("failure err = %v", err)
	}
}

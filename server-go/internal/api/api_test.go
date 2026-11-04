package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stickers-server/internal/apperr"
	"stickers-server/internal/config"
	"stickers-server/internal/media"
	"stickers-server/internal/models"
	"stickers-server/internal/sticker"
	"stickers-server/internal/storage"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

// ---- fakes ----------------------------------------------------------------

// fakeDownloader writes a synthetic 2s 320x240 video as source.mp4, or fails
// with err when set.
type fakeDownloader struct {
	settings config.Settings
	err      error
	calls    int
}

func (f *fakeDownloader) Download(ctx context.Context, jobID, url string) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	return media.RunFFmpeg(ctx, time.Minute, media.Msgs{Timeout: "t", Failed: "f"},
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=25",
		"-pix_fmt", "yuv420p", filepath.Join(f.settings.JobDir(jobID), "source.mp4"))
}

// fakeRenderer records its inputs and writes small placeholder files.
type fakeRenderer struct {
	animatedArgs []float64
	staticArgs   []float64
	crop         sticker.CropPixels
	err          error
}

func (f *fakeRenderer) FitAnimated(_ context.Context, _, out string, start, end float64,
	crop sticker.CropPixels, _, _ int) (models.RenderFitInfo, error) {
	if f.err != nil {
		return models.RenderFitInfo{}, f.err
	}
	f.animatedArgs, f.crop = []float64{start, end}, crop
	fps, q := 12.0, 61
	return models.RenderFitInfo{FPS: &fps, Quality: &q, Degraded: true}, os.WriteFile(out, []byte("ANIM"), 0o644)
}

func (f *fakeRenderer) FitStatic(_ context.Context, _, out string, frameAt float64,
	crop sticker.CropPixels, _, _ int) (models.RenderFitInfo, error) {
	f.staticArgs, f.crop = []float64{frameAt}, crop
	q := 90
	return models.RenderFitInfo{Quality: &q}, os.WriteFile(out, []byte("STILL"), 0o644)
}

func (f *fakeRenderer) RenderTray(_ context.Context, _, out string, _ float64, _ sticker.CropPixels, _, _ int) error {
	return os.WriteFile(out, []byte("TRAY"), 0o644)
}

// ---- harness ----------------------------------------------------------------

type harness struct {
	t    *testing.T
	url  string
	dl   *fakeDownloader
	rend *fakeRenderer
	cfg  config.Settings
}

func newHarness(t *testing.T, tweak func(*config.Settings)) *harness {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	cfg := config.Default()
	cfg.StorageDir = t.TempDir()
	cfg.RateLimitPerHour = 0
	cfg.FFmpegTimeout = time.Minute
	if tweak != nil {
		tweak(&cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	dl := &fakeDownloader{settings: cfg}
	rend := &fakeRenderer{}
	srv := New(ctx, cfg, storage.New(cfg), dl, rend)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); cancel(); srv.Wait() })
	return &harness{t: t, url: ts.URL, dl: dl, rend: rend, cfg: cfg}
}

func (h *harness) do(method, path string, body any, hdr map[string]string) (*http.Response, []byte) {
	h.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, h.url+path, rd)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func (h *harness) json(method, path string, body any, out any) *http.Response {
	h.t.Helper()
	resp, data := h.do(method, path, body, nil)
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			h.t.Fatalf("%s %s: bad JSON %q: %v", method, path, data, err)
		}
	}
	return resp
}

func (h *harness) waitJob(id string, want models.JobStatus) models.JobResponse {
	h.t.Helper()
	var j models.JobResponse
	for i := 0; i < 400; i++ {
		h.json("GET", "/api/v1/jobs/"+id, nil, &j)
		if j.Status == want || j.Status == models.JobFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if j.Status != want {
		h.t.Fatalf("job status = %s (error %+v), want %s", j.Status, j.Error, want)
	}
	return j
}

func (h *harness) waitRender(id string, want models.RenderStatus) models.RenderResponse {
	h.t.Helper()
	var r models.RenderResponse
	for i := 0; i < 400; i++ {
		h.json("GET", "/api/v1/renders/"+id, nil, &r)
		if r.Status == want || r.Status == models.RenderFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if r.Status != want {
		h.t.Fatalf("render status = %s (error %+v), want %s", r.Status, r.Error, want)
	}
	return r
}

const tiktokURL = "https://www.tiktok.com/@u/video/123"

func (h *harness) readyJob() string {
	h.t.Helper()
	var j models.JobResponse
	if resp := h.json("POST", "/api/v1/jobs", map[string]string{"url": tiktokURL}, &j); resp.StatusCode != 200 {
		h.t.Fatalf("create job: %d", resp.StatusCode)
	}
	h.waitJob(j.JobID, models.JobReady)
	return j.JobID
}

func errCode(t *testing.T, data []byte) models.ErrorCode {
	t.Helper()
	var e models.APIError
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("not an error body: %s", data)
	}
	return e.Code
}

// ---- tests ------------------------------------------------------------------

func TestHealthAndCORS(t *testing.T) {
	h := newHarness(t, nil)
	resp, data := h.do("GET", "/health", nil, nil)
	if resp.StatusCode != 200 || !strings.Contains(string(data), "Hello World") {
		t.Fatalf("health: %d %s", resp.StatusCode, data)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
	resp, _ = h.do("OPTIONS", "/api/v1/jobs", nil, map[string]string{
		"Origin": "http://x", "Access-Control-Request-Method": "POST",
	})
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Headers") == "" {
		t.Errorf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
}

func TestCreateJobValidation(t *testing.T) {
	h := newHarness(t, nil)
	cases := []struct {
		name, body string
		status     int
		code       models.ErrorCode
	}{
		{"bad json", `{`, 422, "invalid_request"},
		{"missing url", `{}`, 422, "invalid_request"},
		{"not http url", `{"url":"not a url"}`, 422, "invalid_request"},
		{"foreign host", `{"url":"https://youtube.com/watch?v=1"}`, 400, models.HostNotAllowed},
		{"lookalike host", `{"url":"https://tiktok.com.evil.com/x"}`, 400, models.HostNotAllowed},
	}
	for _, c := range cases {
		resp, data := h.do("POST", "/api/v1/jobs", c.body, nil)
		if resp.StatusCode != c.status || errCode(t, data) != c.code {
			t.Errorf("%s: %d %s", c.name, resp.StatusCode, data)
		}
	}
	if h.dl.calls != 0 {
		t.Error("downloader must not be reached for rejected URLs")
	}
}

func TestJobHappyPath(t *testing.T) {
	h := newHarness(t, nil)
	var created models.JobResponse
	resp := h.json("POST", "/api/v1/jobs", map[string]string{"url": tiktokURL}, &created)
	if resp.StatusCode != 200 || created.Status != models.JobPending || !storage.ValidID(created.JobID) {
		t.Fatalf("create: %d %+v", resp.StatusCode, created)
	}
	if created.ProxyURL != nil {
		t.Error("proxy_url must be null until ready")
	}

	j := h.waitJob(created.JobID, models.JobReady)
	if j.Metadata == nil || j.Metadata.Width != 320 || j.Metadata.Height != 240 || j.Progress != 1 {
		t.Fatalf("metadata = %+v", j)
	}
	if j.ProxyURL == nil || *j.ProxyURL != "/api/v1/jobs/"+created.JobID+"/proxy.mp4" {
		t.Fatalf("proxy_url = %v", j.ProxyURL)
	}

	resp, data := h.do("GET", *j.ProxyURL, nil, nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "video/mp4" || len(data) == 0 {
		t.Fatalf("proxy: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// Seeking needs Range support.
	resp, part := h.do("GET", *j.ProxyURL, nil, map[string]string{"Range": "bytes=0-99"})
	if resp.StatusCode != http.StatusPartialContent || len(part) != 100 {
		t.Errorf("range: %d, %d bytes", resp.StatusCode, len(part))
	}
	resp, data = h.do("GET", *j.SpriteURL, nil, nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || len(data) == 0 {
		t.Errorf("sprite: %d", resp.StatusCode)
	}
}

func TestJobDownloadFailureIsReported(t *testing.T) {
	h := newHarness(t, nil)
	h.dl.err = apperr.New(models.VideoPrivate, "Este video es privado.").WithDetail("d")

	var created, j models.JobResponse
	h.json("POST", "/api/v1/jobs", map[string]string{"url": tiktokURL}, &created)
	for i := 0; i < 200 && j.Status != models.JobFailed; i++ {
		h.json("GET", "/api/v1/jobs/"+created.JobID, nil, &j)
		time.Sleep(25 * time.Millisecond)
	}
	if j.Status != models.JobFailed || j.Error == nil || j.Error.Code != models.VideoPrivate {
		t.Fatalf("job = %+v", j)
	}
}

func TestMissingResourcesAndPathTraversal(t *testing.T) {
	h := newHarness(t, nil)
	for _, p := range []string{
		"/api/v1/jobs/" + strings.Repeat("a", 32),
		"/api/v1/jobs/..%2f..%2fetc",
		"/api/v1/jobs/" + strings.Repeat("a", 32) + "/proxy.mp4",
		"/api/v1/renders/" + strings.Repeat("b", 32),
		"/api/v1/renders/not-an-id/sticker.webp",
	} {
		resp, data := h.do("GET", p, nil, nil)
		if resp.StatusCode != 404 || errCode(t, data) != models.NotFound {
			t.Errorf("%s: %d %s", p, resp.StatusCode, data)
		}
	}
}

func TestRenderFlow(t *testing.T) {
	h := newHarness(t, nil)
	jobID := h.readyJob()
	base := "/api/v1/jobs/" + jobID + "/renders"
	crop := map[string]float64{"x": 0, "y": 0, "width": 1, "height": 1}

	var created models.RenderResponse
	resp := h.json("POST", base, map[string]any{"type": "animated", "start": 0.2, "end": 1.5, "crop": crop}, &created)
	if resp.StatusCode != 200 || created.Status != models.RenderPending || created.StickerURL != nil {
		t.Fatalf("create: %d %+v", resp.StatusCode, created)
	}
	r := h.waitRender(created.RenderID, models.RenderReady)
	if r.Bytes == nil || *r.Bytes != 4 || r.Fit == nil || *r.Fit.Quality != 61 || !r.Fit.Degraded {
		t.Fatalf("render = %+v", r)
	}
	if h.rend.animatedArgs[0] != 0.2 || h.rend.animatedArgs[1] != 1.5 || h.rend.crop.Width != 320 || h.rend.crop.Height != 240 {
		t.Errorf("renderer got %v %+v", h.rend.animatedArgs, h.rend.crop)
	}
	resp, data := h.do("GET", *r.StickerURL, nil, nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/webp" || string(data) != "ANIM" {
		t.Errorf("sticker: %d %q", resp.StatusCode, data)
	}
	if _, data = h.do("GET", *r.TrayURL, nil, nil); string(data) != "TRAY" {
		t.Errorf("tray = %q", data)
	}

	// static: frame_at defaults to start
	h.json("POST", base, map[string]any{"type": "static", "start": 0.7, "crop": crop}, &created)
	h.waitRender(created.RenderID, models.RenderReady)
	if len(h.rend.staticArgs) != 1 || h.rend.staticArgs[0] != 0.7 {
		t.Errorf("static frame_at = %v, want 0.7", h.rend.staticArgs)
	}
}

func TestRenderRangeOutOfBoundsFailsTheRender(t *testing.T) {
	h := newHarness(t, nil)
	jobID := h.readyJob()
	crop := map[string]float64{"x": 0, "y": 0, "width": 1, "height": 1}
	var created, r models.RenderResponse
	h.json("POST", "/api/v1/jobs/"+jobID+"/renders",
		map[string]any{"type": "animated", "start": 0, "end": 9, "crop": crop}, &created)
	for i := 0; i < 100 && r.Status != models.RenderFailed; i++ {
		h.json("GET", "/api/v1/renders/"+created.RenderID, nil, &r)
		time.Sleep(25 * time.Millisecond)
	}
	if r.Status != models.RenderFailed || r.Error == nil || r.Error.Code != models.RangeOutOfBounds {
		t.Fatalf("render = %+v", r)
	}
}

func TestRenderRequestErrors(t *testing.T) {
	h := newHarness(t, nil)
	jobID := h.readyJob()
	crop := `"crop":{"x":0,"y":0,"width":1,"height":1}`

	cases := []struct {
		name, job, body string
		status          int
		code            models.ErrorCode
	}{
		{"end before start", jobID, `{"type":"animated","start":2,"end":1,` + crop + `}`, 422, "invalid_request"},
		{"bad type", jobID, `{"type":"gif","start":0,` + crop + `}`, 422, "invalid_request"},
		{"bad crop", jobID, `{"type":"static","start":0,"crop":{"x":0,"y":0,"width":0,"height":1}}`, 422, "invalid_request"},
		{"unknown job", strings.Repeat("c", 32), `{"type":"static","start":0,` + crop + `}`, 404, models.NotFound},
		{"malformed job id", "not-an-id", `{"type":"static","start":0,` + crop + `}`, 404, models.NotFound},
	}
	for _, c := range cases {
		resp, data := h.do("POST", "/api/v1/jobs/"+c.job+"/renders", c.body, nil)
		if resp.StatusCode != c.status || errCode(t, data) != c.code {
			t.Errorf("%s: %d %s", c.name, resp.StatusCode, data)
		}
	}
}

func TestRenderRejectedWhileJobNotReady(t *testing.T) {
	h := newHarness(t, nil)
	id := storage.NewID()
	store := storage.New(h.cfg)
	store.CreateJobDir(id)
	store.WriteJob(models.JobState{JobID: id, Status: models.JobDownloading})

	resp, data := h.do("POST", "/api/v1/jobs/"+id+"/renders",
		`{"type":"static","start":0,"crop":{"x":0,"y":0,"width":1,"height":1}}`, nil)
	if resp.StatusCode != 400 || errCode(t, data) != models.NotFound {
		t.Fatalf("%d %s", resp.StatusCode, data)
	}
}

func TestRateLimit(t *testing.T) {
	h := newHarness(t, func(c *config.Settings) { c.RateLimitPerHour = 2 })
	post := func(device string) int {
		resp, _ := h.do("POST", "/api/v1/jobs", map[string]string{"url": "https://youtube.com/x"},
			map[string]string{"X-Device-Id": device})
		return resp.StatusCode
	}
	if post("a") != 400 || post("a") != 400 { // rejected by host check, but counted
		t.Fatal("first two requests should reach validation")
	}
	resp, data := h.do("POST", "/api/v1/jobs", map[string]string{"url": "https://youtube.com/x"},
		map[string]string{"X-Device-Id": "a"})
	if resp.StatusCode != 429 || errCode(t, data) != models.RateLimited {
		t.Fatalf("third: %d %s", resp.StatusCode, data)
	}
	if post("b") != 400 {
		t.Error("another device must not be limited")
	}
	// Reads are never rate limited.
	if resp, _ := h.do("GET", "/health", nil, nil); resp.StatusCode != 200 {
		t.Error("health limited")
	}
}

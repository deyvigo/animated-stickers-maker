# Stickers Maker — Go API

Go port of the backend. It serves the same `/api/v1` endpoints and JSON as the Python
version, so the app needs no changes. Downloading is delegated to `../downloader`
(yt-dlp has no Go equivalent); everything else happens here: probe, editor previews
(proxy + sprite) and WhatsApp-compliant rendering (512×512 WebP, animated ≤500 KB or
static ≤100 KB, 96×96 tray icon).

## Run

From the repo root (starts this API and the downloader sharing one storage volume):

```bash
docker compose up --build
curl http://localhost:8000/health
```

## Develop

Needs Go 1.22+ and `ffmpeg`/`ffprobe` on the PATH.

```bash
cd server-go
go test -race ./...
```

Tests that need an ffmpeg built with `libwebp_anim` (e.g. Homebrew's doesn't have it) are
skipped automatically. To run them against the same ffmpeg the container uses:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.24-bookworm sh -c \
  'apt-get update -qq && apt-get install -y -qq ffmpeg >/dev/null && go test ./...'
```

## Configuration

Environment variables or a `.env` file (see `.env.example`). Notable ones:
`RATE_LIMIT_PER_HOUR` (0 disables it), `ALLOWED_HOSTS`, `DOWNLOADER_URL`,
`MAX_CONCURRENT_DOWNLOADS` / `MAX_CONCURRENT_RENDERS`.

## How animated stickers fit the 500 KB budget

Animated WebP has no inter-frame prediction, so size ≈ frames × bytes per frame. Heavy footage
(fur, water, camera motion) can't keep both a high fps and a good quantizer in 500 KB, and below
about q=50 libwebp shows flat blocky patches. `internal/sticker/search.go` therefore spends the
budget in this order: quality (never below `QFloor`, 55), then a light gaussian pre-blur
(σ 0 → 0.6 → 1.0), and only then frame rate (15 → 12 → 10 → 8 → 6 → 5 fps). Within a candidate
it binary-searches the highest quality that still fits. Typical cost is 2–10 ffmpeg runs.

The `fit` field of a render response reports what it chose (`fps`, `quality`, `blur`,
`duration`). Tune `DefaultPolicy` to trade softness for fps: `Blurs` (e.g. add `1.4` for more
fps on very heavy clips, or leave only `0` to never soften) and `QFloor`.

### Letterbox bars

When the crop leaves areas of the 512×512 canvas empty (a portrait video shown whole, or a
crop reaching outside the frame), `PADDING_FILL` decides what goes there:

- `blur` (default): a blurred, darkened copy of the video, like TikTok. Opaque.
- `transparent`: a clean cutout. Real transparency makes libwebp spend about a third more bytes
  on the same frame, which on heavy clips costs several fps.

On a real 5 s clip shown whole (heavy CGI footage) this moved the result from 6 fps / q74 to
15 fps / q65 with a light blur, at the same ~490 KB.

## Layout

```
cmd/server        entrypoint (graceful shutdown, -healthcheck for Docker)
internal/api      routes, validation, background job/render pipelines
internal/sticker  crop math, fps/quality search, WebP rendering
internal/media    ffmpeg/ffprobe runner, probe, previews
internal/webpinfo reads size/duration from a WebP's RIFF container (replaces Pillow)
internal/download host allowlist + client for the downloader service
internal/storage  job.json / render.json on disk, TTL cleanup
internal/{config,models,apperr,queue,ratelimit}
```

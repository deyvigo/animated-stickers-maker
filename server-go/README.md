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

Animated WebP has no inter-frame prediction, so size ≈ frames × bytes per frame. For long
clips the encoder can't keep both high fps and good quality. `internal/sticker/search.go`
searches for the best combination instead of walking a fixed ladder: it keeps the highest fps
that stays at or above a quality floor (`QFloor`, 55), then spends the remaining budget on
quality. Long clips therefore get fewer fps rather than blocky frames. Tune `DefaultPolicy`
to trade smoothness for sharpness.

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

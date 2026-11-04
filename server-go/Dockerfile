# syntax=docker/dockerfile:1
FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM debian:bookworm-slim

# ffmpeg/ffprobe do all the media work (probe, proxy, sprite, WebP encode).
# The Debian build includes libwebp_anim, which the sticker encoder needs.
RUN apt-get update && \
    apt-get install -y --no-install-recommends ffmpeg ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --uid 1000 appuser
COPY --from=build /out/server /usr/local/bin/server

ENV STORAGE_DIR=/data/storage
RUN mkdir -p /data/storage && chown -R appuser:appuser /data
USER appuser

EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/server", "-healthcheck"]

CMD ["/usr/local/bin/server"]

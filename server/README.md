# Stickers Maker — backend

FastAPI + yt-dlp + ffmpeg. Descarga videos de TikTok y genera stickers de WhatsApp
(WebP animado ≤500KB o estático ≤100KB, siempre 512×512) a partir de un recorte de
tiempo + un rectángulo de crop elegidos en la app.

Sin cuentas ni base de datos: cada job vive en `${STORAGE_DIR}/jobs/{id}/` con un TTL
(6h por defecto) y se limpia solo.

## Levantar con Docker (recomendado)

```bash
docker compose up --build
curl http://localhost:8000/health
```

El servicio escucha en `0.0.0.0:8000` — importante para que un celular en la misma red
Wi-Fi pueda llegar a `http://<IP-de-tu-Mac>:8000` (ver `../app/README.md`). Encontrá esa
IP con `ipconfig getifaddr en0`.

Los datos persisten en el volumen nombrado `stickers_storage` entre reinicios del
contenedor. `compose.yaml` monta `./app` de solo lectura para hot-reload en desarrollo.

### Actualizar yt-dlp

TikTok cambia su sitio seguido y rompe el extractor de yt-dlp. Cuando eso pase (errores
`extractor_error` en los jobs), subí la versión fijada en `pyproject.toml` y reconstruí:

```bash
docker compose build --no-cache
```

## Desarrollo local sin Docker (opcional)

Necesita Python 3.11+ y `ffmpeg`/`ffprobe` en el PATH:

```bash
uv run uvicorn app.main:app --reload --host 0.0.0.0 --port 8000
```

## Probar el flujo completo con curl

```bash
JOB=$(curl -s -X POST localhost:8000/api/v1/jobs \
  -H 'content-type: application/json' \
  -d '{"url":"<link de tiktok>"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['job_id'])")

# Esperar a que status pase a "ready" (polling)
curl -s localhost:8000/api/v1/jobs/$JOB

# Generar un sticker animado (crop cuadrado, 2s)
curl -s -X POST localhost:8000/api/v1/jobs/$JOB/renders \
  -H 'content-type: application/json' \
  -d '{"type":"animated","start":1.0,"end":3.0,"crop":{"x":0,"y":0.2,"width":1.0,"height":0.56},"emojis":["😂"]}'

# Descargar el resultado (usar el render_id de la respuesta anterior)
curl -o sticker.webp localhost:8000/api/v1/renders/<render_id>/sticker.webp
ffprobe -v error -show_entries stream=width,height sticker.webp
ls -l sticker.webp   # < 500KB animado / < 100KB estático
```

## Tests

Requieren Python 3.11+ y un ffmpeg con soporte de libwebp (el del contenedor lo tiene; el
de Homebrew en Mac normalmente no — por eso conviene correrlos dentro del contenedor):

```bash
docker compose run --rm --user root -v "$(pwd)/tests:/app/tests:ro" api sh -c \
  "python -m ensurepip --upgrade >/dev/null 2>&1 && \
   python -m pip install --quiet pytest pytest-asyncio httpx && \
   python -m pytest tests -v"
```

## Piezas clave

- `app/services/webp.py` — el "budget fitter": prueba una escalera fija de
  fps/calidad (y si hace falta, recorta la duración) hasta que el WebP entra en el
  límite de WhatsApp. Siempre devuelve qué tuvo que degradar.
- `app/services/download.py` — aísla yt-dlp; valida el host contra una allowlist de
  TikTok antes de descargar nada (evita SSRF / uso como downloader genérico).
- `app/services/probe.py` — valida el WebP de salida con **Pillow**, no con `ffprobe`:
  el ffmpeg de Debian puede *escribir* WebP animado pero su propio decodificador no
  puede leer los chunks ANIM/ANMF de vuelta (falso negativo). WhatsApp y los
  navegadores sí lo leen bien.
- `app/core/queue.py` — concurrencia acotada (semáforos) para descargas y renders.

## Límites de WhatsApp (verificados contra el código fuente de
[WhatsApp/stickers](https://github.com/WhatsApp/stickers))

| | |
|---|---|
| Dimensión | 512×512 exacto |
| Estático | ≤ 100 KB |
| Animado | ≤ 500 KB, < 10000 ms totales, ≥ 8 ms por frame |
| Tray icon | 24–512 px, ≤ 50 KB |
| Stickers por pack | 3 a 30 |
| Emojis por sticker | 1 a 3 |
| Nombre/publisher | solo `[A-Za-z0-9_.,' ]`, sin `..` (ver `app/src/features/packs/types.ts` en la app) |

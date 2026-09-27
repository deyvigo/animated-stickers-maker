# Stickers Maker

Convierte un link de TikTok en un sticker de WhatsApp (animado o foto). Dos partes:

- `server/` — backend FastAPI + yt-dlp + ffmpeg (Docker). Detalles en [`server/README.md`](server/README.md).
- `app/` — app Expo/React Native (Android). Detalles en [`app/README.md`](app/README.md).

Este documento es la guía rápida de punta a punta: levantar el backend y hacer un build
local de la app para instalarla en el celular.

## Requisitos

- Docker (para el backend).
- Node.js y `eas-cli` (`npm i -g eas-cli`, luego `eas login`).
- Para el build **local**: Android SDK + JDK 17 instalados en tu máquina.
- El celular y la Mac en la **misma red Wi-Fi** (la app le pega al backend por la IP local
  de la Mac, no por `localhost`).

## 1. Levantar el backend

```bash
cd server
docker compose up --build
curl http://localhost:8000/health
```

Anotá la IP de tu Mac en la red local, la vas a necesitar en el siguiente paso:

```bash
ipconfig getifaddr en0
```

Más detalle (tests, endpoints, límites de WhatsApp) en [`server/README.md`](server/README.md).

## 2. Configurar la app

```bash
cd app
cp .env.example .env
npm install
```

Editá `.env` y poné la IP de tu Mac (**no** `localhost`, el celular no puede resolverlo):

```
EXPO_PUBLIC_API_URL=http://192.168.1.XXX:8000
```

## 3. Build local del dev client (Android)

`eas build --local` empaqueta el proyecto tal como está en git, y git tiene `.env`
ignorado (ver `app/.gitignore`). Si no se hace nada más, el build sale sin
`EXPO_PUBLIC_API_URL` y la app no va a saber a qué backend llamar. Por eso, **antes de
buildear**, hay que comentar estas dos líneas de `app/.gitignore`:

```gitignore
# .env
# .env*.local
```

(el propio archivo ya lo señala: *"comment this lines when build locally"*). Con eso git
deja de ignorar `.env` y `eas build --local` lo incluye en el paquete.

```bash
npx expo prebuild --platform android
eas build --profile development --platform android --local
```

Instalá el APK resultante en el celular (por ejemplo `adb install build-*.apk`, o copiá el
archivo y abrilo desde el teléfono).

**Después de buildear, volvé a descomentar esas dos líneas en `app/.gitignore`** y
verificá con `git status` que `.env` no haya quedado trackeado ni vaya a subirse en el
próximo commit — tiene tu IP local de red.

## 4. Correr la app

```bash
npx expo start --dev-client
```

Hay que repetir el build (paso 3) cada vez que cambie la IP del backend en `.env`, o cada
vez que se toque el código nativo en `modules/whatsapp-stickers` — Fast Refresh solo
actualiza JS/TS.

## Problemas comunes

- **La app no llega al backend**: revisá que el celular y la Mac estén en la misma
  Wi-Fi, que la IP en `.env` sea la correcta (puede cambiar entre reinicios de router) y
  que el firewall de la Mac no esté bloqueando el puerto 8000.
- **"EXPO_PUBLIC_API_URL no está configurado"** en la app: el build se hizo sin
  comentar las dos líneas de `app/.gitignore`, así que `.env` no entró al paquete. Repetí
  el paso 3.
- **`extractor_error` al crear un job**: TikTok cambió algo y rompió yt-dlp. Ver la
  sección "Actualizar yt-dlp" en [`server/README.md`](server/README.md).

# Stickers Maker

Turns a TikTok link into a WhatsApp sticker (animated or photo). Two parts:

- `server/` — FastAPI + yt-dlp + ffmpeg backend (Docker). Details in [`server/README.md`](server/README.md).
- `app/` — Expo/React Native app (Android). Details in [`app/README.md`](app/README.md).

This document is the quick end-to-end guide: run the backend and produce a local build of
the app to install it on your phone.

## Requirements

- Docker (for the backend).
- Node.js and `eas-cli` (`npm i -g eas-cli`, then `eas login`).
- For the **local** build: Android SDK + JDK 17 installed on your machine.
- Your phone and Mac on the **same Wi-Fi network** (the app reaches the backend via your
  Mac's local IP, not `localhost`).

## 1. Run the backend

```bash
cd server
docker compose up --build
curl http://localhost:8000/health
```

Note your Mac's local network IP, you'll need it in the next step:

```bash
ipconfig getifaddr en0
```

More detail (tests, endpoints, WhatsApp limits) in [`server/README.md`](server/README.md).

## 2. Configure the app

```bash
cd app
cp .env.example .env
npm install
```

Edit `.env` and set your Mac's IP (**not** `localhost` — your phone can't resolve that):

```
EXPO_PUBLIC_API_URL=http://192.168.1.XXX:8000
```

## 3. Local dev client build (Android)

`eas build --local` packages the project exactly as it is in git, and git has `.env`
ignored (see `app/.gitignore`). If nothing else is done, the build comes out without
`EXPO_PUBLIC_API_URL` and the app won't know which backend to call. So, **before
building**, comment out these two lines in `app/.gitignore`:

```gitignore
# .env
# .env*.local
```

(the file itself already flags this: *"comment this lines when build locally"*). That
stops git from ignoring `.env`, so `eas build --local` includes it in the package.

```bash
npx expo prebuild --platform android
eas build --profile development --platform android --local
```

Install the resulting APK on your phone (e.g. `adb install build-*.apk`, or copy the file
over and open it from the phone).

**After building, uncomment those two lines in `app/.gitignore` again** and check with
`git status` that `.env` hasn't been tracked or is about to be committed — it has your
local network IP.

## 4. Run the app

```bash
npx expo start --dev-client
```

You need to repeat the build (step 3) whenever the backend IP in `.env` changes, or
whenever native code in `modules/whatsapp-stickers` is touched — Fast Refresh only
updates JS/TS.

## Common issues

- **The app can't reach the backend**: check that the phone and Mac are on the same
  Wi-Fi, that the IP in `.env` is correct (it can change after a router restart), and
  that the Mac's firewall isn't blocking port 8000.
- **"EXPO_PUBLIC_API_URL no está configurado"** in the app: the build was made without
  commenting out the two lines in `app/.gitignore`, so `.env` didn't make it into the
  package. Repeat step 3.
- **`extractor_error` when creating a job**: TikTok changed something and broke yt-dlp.
  See the "Actualizar yt-dlp" section in [`server/README.md`](server/README.md).

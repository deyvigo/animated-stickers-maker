# Stickers Maker — app móvil

App Expo (React Native + TypeScript) para crear stickers de WhatsApp (animados o de foto)
a partir de un link de TikTok. Ver el plan completo en `../server` para el backend.

Requiere **dev client** (no funciona en Expo Go) porque incluye un módulo nativo propio
(`modules/whatsapp-stickers`) para integrar con WhatsApp.

## 1. Configurar el backend

1. Levantá el backend (ver `../server/README.md`): `docker compose up --build` desde `server/`.
2. Encontrá la IP de tu Mac en la red local:
   ```bash
   ipconfig getifaddr en0
   ```
3. Copiá `.env.example` a `.env` y poné esa IP:
   ```
   EXPO_PUBLIC_API_URL=http://192.168.1.XXX:8000
   ```
   **No uses `localhost`** — tu celular no puede resolverlo a tu Mac. Tu celular y tu Mac
   tienen que estar en la misma red Wi-Fi.

## 2. Build del dev client

```bash
npm install
eas init
eas build:configure
npx expo prebuild --platform android   # genera android/ a partir de app.config.ts
eas build --profile development --platform android   # o: npx expo run:android si tenés el SDK de Android local
eas build --profile development --platform android --local
```

Instalá el APK resultante en tu celular, y arrancá el bundler:

```bash
npx expo start --dev-client
```

Cada vez que cambies algo en `modules/whatsapp-stickers` (código nativo) hay que repetir el
build — Fast Refresh solo actualiza JS/TS.

## 3. Checklist de prueba en el dispositivo

Esto es lo que te toca verificar a vos en tu celular (ver la sección "Verificación" del plan):

- [ ] Pegar un link de TikTok en la pestaña "Nuevo" → se crea el job y pasa a "Recortar".
- [ ] El video se reproduce en loop en el editor, el timeline con miniaturas se ve.
- [ ] Mover los handles de recorte (modo Animado) cambia la duración mostrada.
- [ ] El cuadro de crop se puede arrastrar y redimensionar (handle inferior-derecho).
- [ ] Cambiar a modo "Foto" muestra un único playhead en vez de dos handles.
- [ ] "Generar sticker" → pantalla de progreso → vista previa con el peso final en KB.
- [ ] Elegir 1-3 emojis y guardar en un pack nuevo.
- [ ] Repetir hasta tener 3 stickers del mismo tipo (animado o foto) en un pack.
- [ ] En "Mis packs", el botón "Añadir a WhatsApp" se activa recién con 3 stickers.
- [ ] Tocarlo abre el diálogo de WhatsApp y el pack queda instalado (revisar el teclado de
      stickers de WhatsApp). Marcá algunos como favoritos ahí mismo.
- [ ] Casos borde: WhatsApp no instalado, sin conexión al generar, link inválido.

Si algo de esto falla, decime **qué pasó** (mensaje de error visible, en qué paso) y el
`job_id`/`render_id` si aparece en algún error — así puedo diagnosticar sin tener el
dispositivo delante.

## Estructura

```
src/
  app/                    # rutas (expo-router)
    (tabs)/               # "Nuevo" y "Mis packs"
    editor.tsx            # recorte + crop
    preview.tsx           # resultado + guardar en pack
  features/
    editor/               # TrimTimeline, CropOverlay (Reanimated + Gesture Handler)
    packs/                 # almacenamiento local de packs (expo-file-system) + hooks
  lib/                     # cliente API, react-query, device id
modules/
  whatsapp-stickers/       # módulo nativo Android (Kotlin) para integrar con WhatsApp
```

## Notas conocidas

- La integración con WhatsApp (`modules/whatsapp-stickers`) está verificada contra el
  código fuente público de [WhatsApp/stickers](https://github.com/WhatsApp/stickers), pero
  el chequeo "¿el pack ya está agregado?" usa un endpoint de WhatsApp que ellos no
  documentan formalmente — si `isPackAdded` no funciona como se espera en tu WhatsApp, no
  bloquea el flujo principal (agregar el pack funciona igual), solo afecta el texto del
  botón.
- iOS no está implementado (el módulo nativo tiene stubs que rechazan con un mensaje
  claro) — el proyecto es Android-first según el plan.
- Hay dos errores de tipos preexistentes del template en `src/components/animated-icon.web.tsx`
  y `src/constants/theme.ts` (imports de `.css` sin tipos) — son de la plantilla original de
  `create-expo-app`, no afectan el bundling real con Metro, solo ruido en `tsc --noEmit`.

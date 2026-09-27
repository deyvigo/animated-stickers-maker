# Product

<!-- impeccable:product-schema 1 -->

## Platform

android

## Users

The user is the developer/owner themself, using the app personally (and possibly sharing with friends/family) to turn TikTok clips into WhatsApp sticker packs. Not built for public distribution or app-store audiences at this stage.

## Product Purpose

Turn a TikTok video link into a WhatsApp sticker (animated or still/photo) and add it to a sticker pack that installs directly into WhatsApp. The flow: paste a TikTok URL → job downloads/processes the video → user trims and crops in-app → generates the sticker → previews final size (KB) → tags with 1-3 emojis → saves into a pack → once a pack has 3+ stickers, "Add to WhatsApp" installs it via the native `whatsapp-stickers` module.

## Positioning

The core value is precision editing, not just convenience: the in-app trim timeline (with thumbnails, dual handles for animated mode, single playhead for photo mode) and the draggable/resizable zoom-crop overlay are what make the resulting sticker look and fit exactly right, as opposed to generic "auto-crop" TikTok-to-sticker tools. Speed matters (paste link → sticker) but is secondary to getting the crop/trim right.

## Operating Context

- Mobile-only workflow, phone in hand, personal WhatsApp use.
- Requires a custom Expo dev client (not Expo Go) because of the native `modules/whatsapp-stickers` Android module.
- Depends on a self-hosted FastAPI backend (`server/`) reachable over the local network (LAN, via the Mac's IP — cleartext HTTP in dev) for downloading/processing TikTok video and rendering the final WebP sticker.
- Job lifecycle surfaces real async states the UI must reflect: `pending → downloading → processing → ready` (or `failed`) for the source job, and a separate `pending → rendering → ready/failed` for each sticker render.
- WhatsApp's own sticker pack rules are an operating constraint: a pack needs a minimum of 3 stickers before "Add to WhatsApp" is meaningful/enabled, and the app's "is this pack already added?" check depends on an undocumented WhatsApp endpoint that can silently fail without blocking the core add-to-WhatsApp flow.

## Capabilities and Constraints

- **Android-first is durable**: iOS is not implemented (native module stubs reject with a clear error); do not design as if iOS parity is imminent.
- **Spanish-only is durable**: all UI copy and user-facing error messages are in Spanish; no i18n is planned. Error codes are stable/translatable on the backend (`ErrorCode` enum) specifically so the app can map them to Spanish copy — raw exception text must never reach the user.
- Editor supports two sticker modes: "Animado" (trim range via two handles, produces an animated WebP) and "Foto" (single frame via one playhead, produces a static WebP).
- Crop is a freeform zoom/pan rectangle that may legitimately extend beyond the source video's frame (e.g. default "whole video visible" state for portrait-into-square); the renderer pads out-of-bounds regions with black rather than rejecting them.
- Final sticker weight (KB) is shown to the user post-generation — file size is a visible, real constraint (WhatsApp enforces sticker size limits), not just an internal detail.
- No user accounts/auth; packs are stored locally on-device (`expo-file-system`).
- Edge cases the app must handle visibly: WhatsApp not installed, no connection during generation, invalid/unsupported TikTok link, video too large/private/unavailable.

## Brand Commitments

None established yet. Current icons/theme are unmodified Expo/create-expo-app template defaults (react-logo, expo-logo, generic adaptive-icon placeholders, template light/dark color tokens) — treat as placeholder, not as an incumbent identity to preserve.

## Evidence on Hand

- Backend error taxonomy (`server/app/models/schemas.py`: `ErrorCode`) enumerates every user-facing failure state the UI needs copy for: invalid URL, host not allowed, video private/unavailable/too large, download failed, extractor error, render failed, range out of bounds, rate limited, not found, timeout, internal error.
- No testimonials, case studies, or external proof exist or should be fabricated — this is a pre-release personal project.

## Product Principles

1. Editing precision over automation — never trade away fine-grained trim/crop control for a "simpler" auto-generated result.
2. Design for Android + Material conventions and native WhatsApp integration touchpoints; no iOS accommodations.
3. All copy, labels, and error messages are Spanish-first; never surface raw/English exception text to the user.
4. Make async job/render state (downloading, processing, rendering, failures) legible at every step — the backend is slow and network-dependent by nature (video download + processing).
5. Respect WhatsApp's real constraints (3-sticker minimum, size limits, install flow) as UX truth, not edge cases to hide.

## Accessibility & Inclusion

No product-specific accessibility requirement established yet.

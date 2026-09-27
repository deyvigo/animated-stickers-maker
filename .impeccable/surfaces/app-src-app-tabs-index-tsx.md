---
version: 1
slug: "app-src-app-tabs-index-tsx"
primary_target: "app/src/app/(tabs)/index.tsx"
related_targets: ["app/src/app/(tabs)/packs.tsx","app/src/app/pack-detail.tsx","app/src/app/editor.tsx","app/src/app/preview.tsx","app/src/features/editor/TrimTimeline.tsx","app/src/features/editor/ZoomableVideoCrop.tsx","app/src/components/app-tabs.tsx"]
---

## Scope

The app's full committed visual identity now covers every screen in the two main flows, plus the persistent chrome around them:

- `app/src/app/(tabs)/index.tsx` — Nuevo (paste link → Continuar)
- `app/src/app/editor.tsx` + `app/src/features/editor/TrimTimeline.tsx` + `app/src/features/editor/ZoomableVideoCrop.tsx` — Recortar (trim + crop, mode toggle, Generar sticker)
- `app/src/app/preview.tsx` — Sticker (preview, emojis, save to pack)
- `app/src/app/(tabs)/packs.tsx` — Mis packs (list, add-to-WhatsApp/delete)
- `app/src/app/pack-detail.tsx` — one pack's sticker grid
- `app/src/components/app-tabs.tsx` — the bottom nav bar itself (background, icons, indicator, labels)

Every screen reachable from either tab, and the tab bar wrapping them, is now on the same palette; nothing in the app is left on the old system light/dark theme.

Mode: Operate throughout. On the editor specifically, expression must never obscure the crop boundary, the trim handles, the live playback position, or which mode (Animado/Foto) is active — this screen is the app's own positioning claim (precision editing), so the crop/trim controls carry the accent color deliberately, not as decoration. On the tab bar, the icons must stay identifiable at 24dp and the selected/unselected states must be unambiguous at a glance.

## Audience, job, task, constraints

- Audience: the developer/owner personally, Spanish (voseo, Argentina), Android, at night/casually.
- Job/task on editor: choose Animado or Foto, trim the clip or pick a frame, pinch/pan to crop, generate the sticker; on preview: see the generated sticker + its file size, pick up to 3 emojis, save into an existing or new pack; on the tab bar: always know which of the two tabs is active.
- Constraints: gesture-driven controls (Reanimated + Gesture Handler) and native `VideoView` playback untouched; the transparency checkerboard in the crop viewport (`ZoomableVideoCrop`) is a universal editor convention and was deliberately left un-recolored; typography stays the device default system font everywhere, no bundled font in the app; Spanish-only copy unchanged; all existing behavior (playback sync, render polling, pack creation, tab navigation) untouched. Tab icons are raster PNGs (native `NativeTabs.Trigger.Icon` requires a bitmap source, not a vector component) with `renderingMode="template"` so they're tinted from code, not baked-in color.

## Chosen direction and memorable moment

Same "Kiosco nocturno" direction as every other screen, now complete: the crop viewport's border and every trim-timeline control (range handles, playhead, selection fill) use the mint-cyan accent instead of the old ad-hoc blue/red/white; the live-playback line got a mint-cyan glow instead of an invisible-on-black drop-shadow. "Generar sticker" and "Crear" (new pack) share the same accent-fill button and warming-tube loading pulse as Continuar/Añadir a WhatsApp. The mode toggle (Animado/Foto) became a proper segmented control in the kiosk material. All five headered routes' native top bars are recolored via `kioskHeaderOptions` in `_layout.tsx`.

The bottom tab bar itself was the last unresolved seam (flagged in every prior round) and is now closed: `app-tabs.tsx` no longer reads the system `Colors[scheme]` — its background is pinned to `Kiosk.background`, its active indicator to `Kiosk.border`, and its two tab icons were replaced entirely. The old icons were unmodified Expo template defaults (a generic house and a compass/explore glyph, unrelated to what the tabs actually do); the new ones are hand-authored, on-brand glyphs: a rounded-square-plus for "Nuevo" (the literal action — start something new) and two overlapping rounded squares for "Mis packs" (a stack of saved packs). Each ships as a proper Android Material 3 outline/filled pair (`new.png`/`new-filled.png`, `packs.png`/`packs-filled.png`, each at 1x/2x/3x) via the icon's `{ default, selected }` source object, tinted `Kiosk.textSecondary` unselected and `Kiosk.accent` selected — matching the selected tab's label color too, so the active tab visibly "lights up" mint, same as every primary button in the app.

Memorable moment: the whole persistent chrome (tab bar, all five native headers) and the whole precision-editing apparatus (trim handles, crop border, playback glow) now read as one deliberate object — nothing borrowed, nothing default.

## Unresolved decisions

- `ZoomableVideoCrop`'s checkerboard tile is intentionally unthemed (see constraints above) — flag if that reads as inconsistent once seen on-device.
- The new tab icons were verified via a rendered preview composited at the app's actual colors and sizes (not an on-device screenshot, no emulator available this session) — worth a close look on a real device, especially the "packs" filled icon's overlap silhouette at true 24dp.

## Direction contract

THESIS: This is a kiosk counter at night, not a generic dark-mode form — every screen in both flows, and the tab bar that frames them, now belongs to that one counter, including the precision tools (trim/crop) that are the product's actual reason to exist.

OWN-WORLD: Pure-black ground (`Colors.dark.background`) everywhere, including the tab bar; mint-cyan `#C2E7DA` as the single glowing accent — trim handles, crop-viewport border, active mode-toggle segment, playback-line glow, and now the selected tab icon/label; `Kiosk.surface` (`#141517`) glass-display-case cards with a hairline border; `Kiosk.inset` (`#0F1011`) for anything sitting inside a case; rounded-but-squared corners, never pill-shaped — including the tab icons' own corner language (rounded squares, not circles or pills); system default typography throughout. Every primary action (Continuar, Generar sticker, Añadir a WhatsApp, Crear) shares one accent-fill button and one warming-tube loading pulse; the tab bar shares the same accent for "this is the active one."

STORY: User pastes a link, lands on the editor already primed with a sensible default crop/trim, adjusts it with mint-highlighted precision controls, generates, previews the result with its real file size, tags it, and saves it into a pack, navigating the whole time via a tab bar whose icons and active state are drawn from the same single palette as everything else.

FIRST VIEWPORT: Editor — square crop viewport (mint-cyan border) on black, segmented Animado/Foto control, sprite-backed trim timeline with mint handles/selection, mint "Generar sticker" button. Preview — square display-case sticker preview, KB readout, emoji grid, pack list, "Crear" button matching the same accent. Tab bar — black bar, two on-brand glyphs (add-tile, stacked-cards), the active one filled and mint, the inactive one outlined and dim.

FORM: Kiosco nocturno, direction concept-seed key `9527a969` (mode: operate, assigned index 7) — same seed as every other screen in this app; this round completes the extension to the editor/preview screens and closes out the tab-bar chrome, no new direction round run.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance. The 12 new tab-icon PNGs each carry an embedded provenance note (`impeccable embed-prompt`) recording they're hand-authored procedural glyphs, not AI-generated or sourced.

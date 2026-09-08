/**
 * On-disk schema for locally-stored sticker packs, written to
 * `${Paths.document}/stickers/packs.json`. The Android native module reads
 * this exact same file (see
 * modules/whatsapp-stickers/android/.../StickerPackRepository.kt) to serve
 * WhatsApp's ContentProvider queries — keep the two in sync if this shape
 * changes.
 */
export interface StickerFile {
  imageFile: string;
  emojis: string[];
  accessibilityText?: string;
}

export interface StickerPack {
  identifier: string;
  name: string;
  publisher: string;
  trayImageFile: string;
  animatedStickerPack: boolean;
  publisherEmail: string;
  publisherWebsite: string;
  privacyPolicyWebsite: string;
  licenseAgreementWebsite: string;
  imageDataVersion: string;
  avoidCache: boolean;
  stickers: StickerFile[];
}

export interface PacksFile {
  packs: StickerPack[];
}

export const WHATSAPP_MIN_STICKERS_PER_PACK = 3;
export const WHATSAPP_MAX_STICKERS_PER_PACK = 30;
export const WHATSAPP_MAX_EMOJIS_PER_STICKER = 3;

export function canSubmitToWhatsApp(pack: StickerPack): boolean {
  return (
    pack.stickers.length >= WHATSAPP_MIN_STICKERS_PER_PACK &&
    pack.stickers.length <= WHATSAPP_MAX_STICKERS_PER_PACK
  );
}

/**
 * WhatsApp's own validator (StickerPackValidator.java, `checkStringValidity`)
 * rejects pack name/publisher with anything outside `[\w-.,'\s]+` (ASCII
 * letters/digits/underscore, hyphen, period, comma, apostrophe, whitespace)
 * or containing "..". That means accented letters and emoji — very likely
 * in a pack name someone types — silently fail the whole "add to WhatsApp"
 * intent with a validation_error WhatsApp shows nowhere in its own UI. We
 * sanitize on save so what's stored is always something WhatsApp accepts.
 */
const WHATSAPP_TEXT_PATTERN = /^[\w\-.,'\s]+$/;

export function sanitizePackText(input: string, fallback: string): string {
  const stripped = input
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '') // strip accents (á -> a) instead of dropping the letter
    .replace(/[^\w\-.,'\s]/g, '') // drop anything still outside WhatsApp's allowed set (emoji, etc.)
    .replace(/\.\.+/g, '.')
    .trim();
  return stripped || fallback;
}

export function isWhatsAppSafeText(input: string): boolean {
  return WHATSAPP_TEXT_PATTERN.test(input) && !input.includes('..');
}

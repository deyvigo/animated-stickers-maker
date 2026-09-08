import { Directory, File, Paths } from 'expo-file-system';

import { absoluteUrl } from '@/lib/api';
import {
  PacksFile,
  sanitizePackText,
  StickerPack,
  WHATSAPP_MAX_STICKERS_PER_PACK,
} from '@/features/packs/types';

const STICKERS_ROOT = 'stickers';
const PACKS_FILE_NAME = 'packs.json';

function stickersDir(): Directory {
  const dir = new Directory(Paths.document, STICKERS_ROOT);
  if (!dir.exists) dir.create({ intermediates: true });
  return dir;
}

function packsFile(): File {
  const file = new File(stickersDir(), PACKS_FILE_NAME);
  if (!file.exists) {
    file.create();
    file.write(JSON.stringify({ packs: [] } satisfies PacksFile));
  }
  return file;
}

function packDir(identifier: string): Directory {
  const dir = new Directory(stickersDir(), identifier);
  if (!dir.exists) dir.create({ intermediates: true });
  return dir;
}

function readPacksFile(): PacksFile {
  try {
    const raw = packsFile().textSync();
    const parsed = JSON.parse(raw) as PacksFile;
    return Array.isArray(parsed.packs) ? parsed : { packs: [] };
  } catch {
    return { packs: [] };
  }
}

function writePacksFile(data: PacksFile): void {
  packsFile().write(JSON.stringify(data, null, 2));
}

export function listPacks(): StickerPack[] {
  return readPacksFile().packs;
}

export function getPack(identifier: string): StickerPack | undefined {
  return listPacks().find((p) => p.identifier === identifier);
}

function newIdentifier(): string {
  return `pack_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
}

export interface CreatePackInput {
  name: string;
  publisher: string;
  animated: boolean;
}

export function createPack(input: CreatePackInput): StickerPack {
  const identifier = newIdentifier();
  packDir(identifier); // ensure the directory exists on disk
  const pack: StickerPack = {
    identifier,
    name: sanitizePackText(input.name, 'Mis stickers'),
    publisher: sanitizePackText(input.publisher, 'Yo'),
    trayImageFile: '',
    animatedStickerPack: input.animated,
    publisherEmail: '',
    publisherWebsite: '',
    privacyPolicyWebsite: '',
    licenseAgreementWebsite: '',
    imageDataVersion: '1',
    avoidCache: false,
    stickers: [],
  };
  const data = readPacksFile();
  data.packs.push(pack);
  writePacksFile(data);
  return pack;
}

export function deletePack(identifier: string): void {
  const data = readPacksFile();
  data.packs = data.packs.filter((p) => p.identifier !== identifier);
  writePacksFile(data);
  const dir = new Directory(stickersDir(), identifier);
  if (dir.exists) dir.delete();
}

export interface AddStickerInput {
  packId: string;
  /** Absolute or API-relative URL to the rendered sticker.webp */
  stickerUrl: string;
  /** Absolute or API-relative URL to the rendered tray.webp */
  trayUrl: string;
  emojis: string[];
}

/** Downloads a render's sticker + tray into the pack's directory and
 * records it in packs.json. Rejects if the pack is already at WhatsApp's
 * 30-sticker cap. */
export async function addStickerToPack(input: AddStickerInput): Promise<StickerPack> {
  const data = readPacksFile();
  const pack = data.packs.find((p) => p.identifier === input.packId);
  if (!pack) {
    throw new Error(`No existe el pack ${input.packId}`);
  }
  if (pack.stickers.length >= WHATSAPP_MAX_STICKERS_PER_PACK) {
    throw new Error(`El pack ya tiene el máximo de ${WHATSAPP_MAX_STICKERS_PER_PACK} stickers.`);
  }

  const dir = packDir(pack.identifier);
  const stickerFileName = `sticker_${pack.stickers.length + 1}.webp`;
  const stickerFile = new File(dir, stickerFileName);
  if (stickerFile.exists) stickerFile.delete();

  const stickerUrl = absoluteUrl(input.stickerUrl) ?? input.stickerUrl;
  const trayUrl = absoluteUrl(input.trayUrl) ?? input.trayUrl;

  await File.downloadFileAsync(stickerUrl, stickerFile);

  // First sticker in a pack also sets the pack's tray icon; later ones
  // don't override it, so the pack keeps a stable icon in WhatsApp's list.
  if (!pack.trayImageFile) {
    const trayFile = new File(dir, 'tray.webp');
    if (trayFile.exists) trayFile.delete();
    await File.downloadFileAsync(trayUrl, trayFile);
    pack.trayImageFile = trayFile.name;
  }

  pack.stickers.push({ imageFile: stickerFileName, emojis: input.emojis.slice(0, 3) });
  writePacksFile(data);
  return pack;
}

export function removeSticker(packId: string, imageFile: string): StickerPack | undefined {
  const data = readPacksFile();
  const pack = data.packs.find((p) => p.identifier === packId);
  if (!pack) return undefined;
  pack.stickers = pack.stickers.filter((s) => s.imageFile !== imageFile);
  writePacksFile(data);
  const file = new File(packDir(packId), imageFile);
  if (file.exists) file.delete();
  return pack;
}

/** file:// URI for a sticker/tray image already saved on disk, for <Image>. */
export function packAssetUri(packId: string, fileName: string): string {
  return new File(packDir(packId), fileName).uri;
}

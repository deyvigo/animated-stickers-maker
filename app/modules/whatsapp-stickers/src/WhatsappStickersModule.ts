import { NativeModule, requireNativeModule } from 'expo';

import { AddPackResult, WhatsAppInstalledResult } from './WhatsappStickers.types';

declare class WhatsappStickersModule extends NativeModule<{}> {
  /** Whether WhatsApp (consumer) and/or WhatsApp Business are installed. */
  isWhatsAppInstalled(): Promise<WhatsAppInstalledResult>;

  /**
   * Best-effort check for whether a pack is already added in WhatsApp.
   * Queries WhatsApp's own (undocumented, historically stable) whitelist
   * provider — see StickerContentProvider.kt's companion doc comment.
   * Returns false on any failure rather than throwing, since this is only
   * used to decide button copy ("Añadir" vs "Ya agregado").
   */
  isPackAdded(packId: string): Promise<boolean>;

  /**
   * Launches WhatsApp's "add sticker pack" flow via the
   * ENABLE_STICKER_PACK intent. Resolves 'added' if WhatsApp reported
   * success, 'cancelled' if the user backed out. Rejects with a message
   * from WhatsApp (e.g. a file that fails its own validation) otherwise.
   */
  addPackToWhatsApp(packId: string): Promise<AddPackResult>;
}

export default requireNativeModule<WhatsappStickersModule>('WhatsappStickers');

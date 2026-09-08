import { NativeModule, registerWebModule } from 'expo';

import { AddPackResult, WhatsAppInstalledResult } from './WhatsappStickers.types';

/** Web has no WhatsApp app to talk to — every call reports "not available"
 * instead of crashing, so the app degrades gracefully when opened in a
 * browser (this project targets Android first; web is just for quick UI
 * iteration on the editor). */
class WhatsappStickersModule extends NativeModule<{}> {
  async isWhatsAppInstalled(): Promise<WhatsAppInstalledResult> {
    return { consumer: false, business: false };
  }

  async isPackAdded(_packId: string): Promise<boolean> {
    return false;
  }

  async addPackToWhatsApp(_packId: string): Promise<AddPackResult> {
    throw new Error('Agregar a WhatsApp solo está disponible en Android.');
  }
}

export default registerWebModule(WhatsappStickersModule, 'WhatsappStickersModule');

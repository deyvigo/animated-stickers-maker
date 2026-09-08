/**
 * `EXPO_PUBLIC_*` vars are inlined at build time by Expo/Metro, so this
 * must point at your Mac's LAN IP (e.g. http://192.168.1.100:8000), not
 * localhost — a physical phone can't resolve "localhost" to your computer.
 * See ../../.env.example and the server's README for how to find that IP.
 */
export const API_BASE_URL = (process.env.EXPO_PUBLIC_API_URL ?? '').replace(/\/+$/, '');

export function assertApiConfigured(): void {
  if (!API_BASE_URL) {
    throw new Error(
      'EXPO_PUBLIC_API_URL no está configurado. Copiá .env.example a .env y ' +
        'poné la IP de tu Mac en la red local (ver README).',
    );
  }
}

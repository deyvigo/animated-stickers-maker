import { File, Paths } from 'expo-file-system';

/**
 * A random id persisted on-device, sent as X-Device-Id so the backend's
 * per-device rate limit (see server/app/core/ratelimit.py) has something
 * more stable than IP alone to key on. Not an auth mechanism — the backend
 * has no accounts.
 */
let cached: string | null = null;

function randomId(): string {
  // No crypto.randomUUID() guarantee across RN/Hermes versions; this only
  // needs to be unique-enough per install, not cryptographically random.
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 12)}`;
}

export function getDeviceId(): string {
  if (cached) return cached;

  const file = new File(Paths.document, 'device_id.txt');
  if (file.exists) {
    const existing = file.textSync().trim();
    if (existing) {
      cached = existing;
      return cached;
    }
  }

  const id = randomId();
  if (!file.exists) file.create();
  file.write(id);
  cached = id;
  return id;
}

/**
 * Thin client for the FastAPI backend (see ../../../server/app/models/schemas.py
 * — these types mirror those Pydantic models 1:1).
 */
import { API_BASE_URL, assertApiConfigured } from '@/lib/env';
import { getDeviceId } from '@/lib/device-id';

export type JobStatus = 'pending' | 'downloading' | 'processing' | 'ready' | 'failed';
export type RenderStatus = 'pending' | 'rendering' | 'ready' | 'failed';
export type StickerType = 'animated' | 'static';

export type ErrorCode =
  | 'invalid_url'
  | 'host_not_allowed'
  | 'video_private'
  | 'video_unavailable'
  | 'video_too_large'
  | 'download_failed'
  | 'extractor_error'
  | 'render_failed'
  | 'range_out_of_bounds'
  | 'rate_limited'
  | 'not_found'
  | 'timeout'
  | 'invalid_request'
  | 'internal_error';

export interface ApiError {
  code: ErrorCode;
  message: string;
  detail?: string | null;
}

export interface JobMetadata {
  duration: number;
  width: number;
  height: number;
  fps: number;
}

export interface Job {
  job_id: string;
  status: JobStatus;
  progress: number;
  metadata: JobMetadata | null;
  proxy_url: string | null;
  sprite_url: string | null;
  error: ApiError | null;
}

export interface CropRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface CreateRenderInput {
  type: StickerType;
  start: number;
  end?: number;
  frame_at?: number;
  crop: CropRect;
  emojis?: string[];
}

export interface RenderFitInfo {
  fps: number | null;
  quality: number | null;
  duration: number | null;
  degraded: boolean;
  note: string | null;
}

export interface Render {
  render_id: string;
  job_id: string;
  status: RenderStatus;
  type: StickerType;
  bytes: number | null;
  sticker_url: string | null;
  tray_url: string | null;
  fit: RenderFitInfo | null;
  error: ApiError | null;
}

/** Thrown for both network failures and structured API errors, so callers
 * can pattern-match on `.code` to show translated, specific copy instead of
 * a generic "algo salió mal". */
export class ApiRequestError extends Error {
  code: ErrorCode;
  detail?: string | null;
  status?: number;

  constructor(error: ApiError, status?: number) {
    super(error.message);
    this.code = error.code;
    this.detail = error.detail;
    this.status = status;
  }
}

// Every endpoint here just writes state and kicks off a background task
// (see server/app/api/jobs.py, renders.py) — the actual yt-dlp/ffmpeg work
// never blocks the HTTP response. So a request that's still pending after
// this long isn't "still working", it's stuck (dropped Wi-Fi, phone
// switched networks, backend mid-restart) — without a timeout, `fetch`
// hangs forever and the caller's loading spinner never clears.
const REQUEST_TIMEOUT_MS = 20_000;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  assertApiConfigured();
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      ...init,
      signal: controller.signal,
      headers: {
        'content-type': 'application/json',
        'x-device-id': getDeviceId(),
        ...init?.headers,
      },
    });
  } catch (cause) {
    if (cause instanceof Error && cause.name === 'AbortError') {
      throw new ApiRequestError({
        code: 'timeout',
        message: 'El servidor no respondió a tiempo. Revisá tu conexión y probá de nuevo.',
        detail: String(cause),
      });
    }
    throw new ApiRequestError({
      code: 'internal_error',
      message:
        'No se pudo conectar con el servidor. Revisá que el backend esté corriendo y que ' +
        'EXPO_PUBLIC_API_URL apunte a la IP de tu Mac en la red local.',
      detail: String(cause),
    });
  } finally {
    clearTimeout(timeoutId);
  }

  if (!response.ok) {
    const fallback: ApiError = {
      code: 'internal_error',
      message: `Error del servidor (HTTP ${response.status}).`,
    };
    let apiError: ApiError;
    try {
      const body = await response.json();
      // Defensive: if the body doesn't have a non-empty `message` (an
      // unexpected shape from some future endpoint, a proxy's own error
      // page, etc.), fall back instead of surfacing a blank error to the
      // user — this is exactly what silently swallowed a 422 before.
      apiError = body && typeof body.message === 'string' && body.message ? body : fallback;
    } catch {
      apiError = fallback;
    }
    throw new ApiRequestError(apiError, response.status);
  }

  return response.json() as Promise<T>;
}

export function absoluteUrl(path: string | null): string | null {
  if (!path) return null;
  return `${API_BASE_URL}${path}`;
}

export const api = {
  createJob(url: string): Promise<Job> {
    return request<Job>('/api/v1/jobs', { method: 'POST', body: JSON.stringify({ url }) });
  },
  getJob(jobId: string): Promise<Job> {
    return request<Job>(`/api/v1/jobs/${jobId}`);
  },
  createRender(jobId: string, input: CreateRenderInput): Promise<Render> {
    return request<Render>(`/api/v1/jobs/${jobId}/renders`, {
      method: 'POST',
      body: JSON.stringify(input),
    });
  },
  getRender(renderId: string): Promise<Render> {
    return request<Render>(`/api/v1/renders/${renderId}`);
  },
};

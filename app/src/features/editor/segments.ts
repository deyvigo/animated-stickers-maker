/** One [start, end) piece of the source video, in seconds. */
export interface Segment {
  start: number;
  end: number;
}

export const MIN_SEGMENT_SECONDS = 0.5;
export const MAX_SEGMENTS = 8; // keep in sync with models.MaxSegments (server)
const NEW_SEGMENT_SECONDS = 1;

export function totalSeconds(segments: Segment[]): number {
  return segments.reduce((sum, s) => sum + (s.end - s.start), 0);
}

/** Seconds that segment `index` may still grow to, given the others use up
 * part of the shared `maxTotal` budget. */
export function maxLengthFor(segments: Segment[], index: number, maxTotal: number): number {
  const others = totalSeconds(segments) - (segments[index].end - segments[index].start);
  return Math.max(MIN_SEGMENT_SECONDS, maxTotal - others);
}

/** Free room around segment `index`: it can't cross its neighbours. */
export function boundsFor(
  segments: Segment[],
  index: number,
  durationSec: number,
): { lower: number; upper: number } {
  return {
    lower: index > 0 ? segments[index - 1].end : 0,
    upper: index < segments.length - 1 ? segments[index + 1].start : durationSec,
  };
}

/** Inserts a new segment (≤1s) centered in the largest free gap that fits
 * both the minimum length and what is left of the total budget. Returns
 * null when there is no room. Keeps the list sorted by start. */
export function addSegment(
  segments: Segment[],
  durationSec: number,
  maxTotal: number,
): { segments: Segment[]; index: number } | null {
  if (segments.length >= MAX_SEGMENTS) return null;
  const budget = maxTotal - totalSeconds(segments);
  if (budget < MIN_SEGMENT_SECONDS) return null;

  let best: { start: number; end: number } | null = null;
  let cursor = 0;
  for (const s of [...segments, { start: durationSec, end: durationSec }]) {
    if (!best || s.start - cursor > best.end - best.start) best = { start: cursor, end: s.start };
    cursor = s.end;
  }
  if (!best || best.end - best.start < MIN_SEGMENT_SECONDS) return null;

  const gap = best.end - best.start;
  const length = Math.min(NEW_SEGMENT_SECONDS, gap, budget);
  const start = best.start + (gap - length) / 2;
  const next = [...segments, { start, end: start + length }].sort((a, b) => a.start - b.start);
  return { segments: next, index: next.findIndex((s) => s.start === start) };
}

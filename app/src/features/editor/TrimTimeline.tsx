import { Image } from 'expo-image';
import { useCallback, useState } from 'react';
import { LayoutChangeEvent, Pressable, StyleSheet, Text, View } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, {
  runOnJS,
  SharedValue,
  useAnimatedStyle,
  useSharedValue,
} from 'react-native-reanimated';

import { Kiosk, Spacing } from '@/constants/theme';

import {
  boundsFor,
  maxLengthFor,
  MIN_SEGMENT_SECONDS,
  Segment,
  totalSeconds,
} from './segments';

const HANDLE_WIDTH = 20;
const TRACK_HEIGHT = 64;
const MIN_STATIC_GAP_PX = 4;

function clamp(value: number, min: number, max: number): number {
  'worklet';
  return Math.min(Math.max(value, min), max);
}

interface AnimatedRangeProps {
  mode: 'animated';
  durationSec: number;
  spriteUri: string | null;
  /** Pieces joined into the sticker, sorted by start. */
  segments: Segment[];
  /** The segment whose handles are shown and editable. */
  activeIndex: number;
  onSelect: (index: number) => void;
  /** Budget shared by all segments together. */
  maxTotalSeconds: number;
  minClipSeconds?: number;
  /** Playback speed multiplier; the sticker lasts total / speed. */
  speed?: number;
  /** Fired with the new bounds of the active segment. */
  onChange: (start: number, end: number) => void;
  /** Live playback position (seconds), updated ~10x/sec from the video
   * player's timeUpdate event. A shared value so it drives the moving line
   * on the UI thread without re-rendering React on every tick. */
  playbackTime?: SharedValue<number>;
  /** Fired once when a handle drag ends (not on every frame of the drag) —
   * used to restart playback from the new `start` so it doesn't sit
   * wherever it happened to be when the range moved. */
  onDragEnd?: () => void;
}

interface StaticFrameProps {
  mode: 'static';
  durationSec: number;
  spriteUri: string | null;
  frameAt: number;
  onChange: (frameAt: number) => void;
}

type TrimTimelineProps = AnimatedRangeProps | StaticFrameProps;

/** Trim control for the editor: a sprite-backed horizontal track with
 * either two draggable handles (animated clip range) or a single playhead
 * (static frame pick). Positions are tracked in pixels via Reanimated for
 * smooth dragging, converted to/from seconds against the measured track
 * width. */
export function TrimTimeline(props: TrimTimelineProps) {
  const [trackWidth, setTrackWidth] = useState(0);
  const usableWidth = Math.max(1, trackWidth - HANDLE_WIDTH);

  const onLayout = useCallback((e: LayoutChangeEvent) => {
    setTrackWidth(e.nativeEvent.layout.width);
  }, []);

  const secToPx = useCallback(
    (sec: number) => (props.durationSec > 0 ? (sec / props.durationSec) * usableWidth : 0),
    [props.durationSec, usableWidth],
  );
  const pxToSec = useCallback(
    (px: number) => (usableWidth > 0 ? (px / usableWidth) * props.durationSec : 0),
    [props.durationSec, usableWidth],
  );

  return (
    <View style={styles.container}>
      <Text style={styles.hint}>
        {props.mode === 'animated' ? animatedHint(props) : `Cuadro en ${props.frameAt.toFixed(1)}s`}
      </Text>
      <View style={styles.track} onLayout={onLayout}>
        {props.spriteUri && (
          <Image source={{ uri: props.spriteUri }} style={StyleSheet.absoluteFill} contentFit="cover" />
        )}
        {trackWidth > 0 &&
          (props.mode === 'animated' ? (
            <>
              {props.segments.map((seg, i) =>
                i === props.activeIndex ? null : (
                  <Pressable
                    key={`${i}-${seg.start}`}
                    onPress={() => props.onSelect(i)}
                    style={[
                      styles.selection,
                      styles.inactiveSelection,
                      {
                        left: secToPx(seg.start) + HANDLE_WIDTH / 2,
                        width: Math.max(0, secToPx(seg.end) - secToPx(seg.start)),
                      },
                    ]}
                  />
                ),
              )}
              <RangeHandles
                key={`${trackWidth}-${props.activeIndex}`}
                usableWidth={usableWidth}
                start={props.segments[props.activeIndex].start}
                end={props.segments[props.activeIndex].end}
                lowerSec={boundsFor(props.segments, props.activeIndex, props.durationSec).lower}
                upperSec={boundsFor(props.segments, props.activeIndex, props.durationSec).upper}
                maxClipSeconds={maxLengthFor(props.segments, props.activeIndex, props.maxTotalSeconds)}
                minClipSeconds={props.minClipSeconds ?? MIN_SEGMENT_SECONDS}
                durationSec={props.durationSec}
                secToPx={secToPx}
                pxToSec={pxToSec}
                onChange={props.onChange}
                playbackTime={props.playbackTime}
                onDragEnd={props.onDragEnd}
              />
            </>
          ) : (
            <Playhead
              key={trackWidth}
              usableWidth={usableWidth}
              frameAt={props.frameAt}
              secToPx={secToPx}
              pxToSec={pxToSec}
              onChange={props.onChange}
            />
          ))}
      </View>
    </View>
  );
}

function animatedHint(props: AnimatedRangeProps): string {
  const total = totalSeconds(props.segments);
  const speed = props.speed ?? 1;
  const count = props.segments.length > 1 ? `${props.segments.length} tramos · ` : '';
  const head = `${count}${total.toFixed(1)}s seleccionados`;
  return speed !== 1
    ? `${head} · dura ${(total / speed).toFixed(1)}s a x${speed}`
    : `${head} (máx. ${props.maxTotalSeconds}s)`;
}

function RangeHandles({
  usableWidth,
  start,
  end,
  lowerSec,
  upperSec,
  maxClipSeconds,
  minClipSeconds,
  durationSec,
  secToPx,
  pxToSec,
  onChange,
  playbackTime,
  onDragEnd,
}: {
  usableWidth: number;
  start: number;
  end: number;
  /** The active segment can't cross its neighbours (or the video's ends). */
  lowerSec: number;
  upperSec: number;
  maxClipSeconds: number;
  minClipSeconds: number;
  durationSec: number;
  secToPx: (s: number) => number;
  pxToSec: (p: number) => number;
  onChange: (start: number, end: number) => void;
  playbackTime?: SharedValue<number>;
  onDragEnd?: () => void;
}) {
  const startX = useSharedValue(secToPx(start));
  const endX = useSharedValue(secToPx(end));
  const minGapPx = secToPx(minClipSeconds);
  const maxGapPx = secToPx(maxClipSeconds);
  const lowerPx = secToPx(lowerSec);
  const upperPx = Math.min(usableWidth, secToPx(upperSec));

  const commit = useCallback(
    (nextStartPx: number, nextEndPx: number) => {
      onChange(pxToSec(nextStartPx), pxToSec(nextEndPx));
    },
    [onChange, pxToSec],
  );

  const notifyDragEnd = useCallback(() => onDragEnd?.(), [onDragEnd]);

  const startGesture = Gesture.Pan()
    .onChange((e) => {
      const min = Math.max(lowerPx, endX.value - maxGapPx);
      const max = endX.value - minGapPx;
      startX.value = clamp(startX.value + e.changeX, min, max);
      runOnJS(commit)(startX.value, endX.value);
    })
    .onEnd(() => runOnJS(notifyDragEnd)());

  const endGesture = Gesture.Pan()
    .onChange((e) => {
      const min = startX.value + minGapPx;
      const max = Math.min(upperPx, startX.value + maxGapPx);
      endX.value = clamp(endX.value + e.changeX, min, max);
      runOnJS(commit)(startX.value, endX.value);
    })
    .onEnd(() => runOnJS(notifyDragEnd)());

  // Dragging the middle of the selection slides the whole range, keeping its
  // length, clamped to the track bounds.
  const moveGesture = Gesture.Pan()
    .onChange((e) => {
      const delta = clamp(e.changeX, lowerPx - startX.value, upperPx - endX.value);
      startX.value += delta;
      endX.value += delta;
      runOnJS(commit)(startX.value, endX.value);
    })
    .onEnd(() => runOnJS(notifyDragEnd)());

  const startHandleStyle = useAnimatedStyle(() => ({ transform: [{ translateX: startX.value }] }));
  const endHandleStyle = useAnimatedStyle(() => ({ transform: [{ translateX: endX.value }] }));
  const selectionStyle = useAnimatedStyle(() => ({
    left: startX.value + HANDLE_WIDTH / 2,
    width: Math.max(0, endX.value - startX.value),
  }));

  // Moving line showing where playback currently is. Playback hops across
  // all segments, so it is only clamped to the track.
  const playbackStyle = useAnimatedStyle(() => {
    if (!playbackTime || durationSec <= 0) return { opacity: 0 };
    const px = (playbackTime.value / durationSec) * usableWidth;
    return {
      opacity: 1,
      transform: [{ translateX: HANDLE_WIDTH / 2 + clamp(px, 0, usableWidth) }],
    };
  });

  return (
    <>
      <GestureDetector gesture={moveGesture}>
        <Animated.View style={[styles.selection, selectionStyle]} />
      </GestureDetector>
      <Animated.View pointerEvents="none" style={[styles.playbackLine, playbackStyle]} />
      <GestureDetector gesture={startGesture}>
        <Animated.View style={[styles.handle, startHandleStyle]} />
      </GestureDetector>
      <GestureDetector gesture={endGesture}>
        <Animated.View style={[styles.handle, endHandleStyle]} />
      </GestureDetector>
    </>
  );
}

function Playhead({
  usableWidth,
  frameAt,
  secToPx,
  pxToSec,
  onChange,
}: {
  usableWidth: number;
  frameAt: number;
  secToPx: (s: number) => number;
  pxToSec: (p: number) => number;
  onChange: (frameAt: number) => void;
}) {
  const x = useSharedValue(secToPx(frameAt));

  const commit = useCallback((px: number) => onChange(pxToSec(px)), [onChange, pxToSec]);

  const gesture = Gesture.Pan().onChange((e) => {
    x.value = clamp(x.value + e.changeX, 0, usableWidth);
    runOnJS(commit)(x.value);
  });

  const style = useAnimatedStyle(() => ({ transform: [{ translateX: x.value }] }));

  return (
    <GestureDetector gesture={gesture}>
      <Animated.View style={[styles.handle, styles.playhead, style]} />
    </GestureDetector>
  );
}

const styles = StyleSheet.create({
  container: {
    gap: Spacing.two,
  },
  track: {
    height: TRACK_HEIGHT,
    borderRadius: Spacing.two,
    overflow: 'hidden',
    backgroundColor: Kiosk.inset,
    borderWidth: 1,
    borderColor: Kiosk.border,
  },
  selection: {
    position: 'absolute',
    top: 0,
    bottom: 0,
    backgroundColor: 'rgba(194, 231, 218, 0.22)',
    borderColor: Kiosk.accent,
    borderTopWidth: 3,
    borderBottomWidth: 3,
    minWidth: MIN_STATIC_GAP_PX,
  },
  inactiveSelection: {
    backgroundColor: 'rgba(194, 231, 218, 0.12)',
    borderColor: Kiosk.border,
  },
  handle: {
    position: 'absolute',
    top: 0,
    bottom: 0,
    width: HANDLE_WIDTH,
    borderRadius: Spacing.one,
    backgroundColor: Kiosk.accent,
    borderWidth: 2,
    borderColor: Kiosk.onAccent,
  },
  playbackLine: {
    position: 'absolute',
    top: -4,
    bottom: -4,
    width: 3,
    marginLeft: -1.5,
    borderRadius: 2,
    backgroundColor: Kiosk.text,
    shadowColor: Kiosk.accent,
    shadowOpacity: 0.9,
    shadowRadius: 4,
    shadowOffset: { width: 0, height: 0 },
  },
  playhead: {
    width: 4,
    marginLeft: HANDLE_WIDTH / 2 - 2,
    backgroundColor: Kiosk.accent,
  },
  hint: {
    textAlign: 'center',
    fontSize: 13,
    color: Kiosk.textSecondary,
  },
});

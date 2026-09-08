import { Image } from 'expo-image';
import { useCallback, useState } from 'react';
import { LayoutChangeEvent, StyleSheet, View } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, {
  runOnJS,
  useAnimatedStyle,
  useSharedValue,
} from 'react-native-reanimated';

import { ThemedText } from '@/components/themed-text';
import { Spacing } from '@/constants/theme';

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
  start: number;
  end: number;
  maxClipSeconds: number;
  minClipSeconds?: number;
  onChange: (start: number, end: number) => void;
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
      <View style={styles.track} onLayout={onLayout}>
        {props.spriteUri && (
          <Image source={{ uri: props.spriteUri }} style={StyleSheet.absoluteFill} contentFit="cover" />
        )}
        {trackWidth > 0 &&
          (props.mode === 'animated' ? (
            <RangeHandles
              key={trackWidth}
              usableWidth={usableWidth}
              start={props.start}
              end={props.end}
              maxClipSeconds={props.maxClipSeconds}
              minClipSeconds={props.minClipSeconds ?? 0.5}
              durationSec={props.durationSec}
              secToPx={secToPx}
              pxToSec={pxToSec}
              onChange={props.onChange}
            />
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
      <ThemedText type="small" themeColor="textSecondary" style={styles.hint}>
        {props.mode === 'animated'
          ? `${(props.end - props.start).toFixed(1)}s seleccionados (máx. ${props.maxClipSeconds}s)`
          : `Cuadro en ${props.frameAt.toFixed(1)}s`}
      </ThemedText>
    </View>
  );
}

function RangeHandles({
  usableWidth,
  start,
  end,
  maxClipSeconds,
  minClipSeconds,
  durationSec,
  secToPx,
  pxToSec,
  onChange,
}: {
  usableWidth: number;
  start: number;
  end: number;
  maxClipSeconds: number;
  minClipSeconds: number;
  durationSec: number;
  secToPx: (s: number) => number;
  pxToSec: (p: number) => number;
  onChange: (start: number, end: number) => void;
}) {
  const startX = useSharedValue(secToPx(start));
  const endX = useSharedValue(secToPx(end));
  const minGapPx = secToPx(minClipSeconds);
  const maxGapPx = secToPx(maxClipSeconds);

  const commit = useCallback(
    (nextStartPx: number, nextEndPx: number) => {
      onChange(pxToSec(nextStartPx), pxToSec(nextEndPx));
    },
    [onChange, pxToSec],
  );

  const startGesture = Gesture.Pan().onChange((e) => {
    const min = Math.max(0, endX.value - maxGapPx);
    const max = endX.value - minGapPx;
    startX.value = clamp(startX.value + e.changeX, min, max);
    runOnJS(commit)(startX.value, endX.value);
  });

  const endGesture = Gesture.Pan().onChange((e) => {
    const min = startX.value + minGapPx;
    const max = Math.min(usableWidth, startX.value + maxGapPx);
    endX.value = clamp(endX.value + e.changeX, min, max);
    runOnJS(commit)(startX.value, endX.value);
  });

  const startHandleStyle = useAnimatedStyle(() => ({ transform: [{ translateX: startX.value }] }));
  const endHandleStyle = useAnimatedStyle(() => ({ transform: [{ translateX: endX.value }] }));
  const selectionStyle = useAnimatedStyle(() => ({
    left: startX.value + HANDLE_WIDTH / 2,
    width: Math.max(0, endX.value - startX.value),
  }));

  return (
    <>
      <Animated.View pointerEvents="none" style={[styles.selection, selectionStyle]} />
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
    backgroundColor: '#00000022',
  },
  selection: {
    position: 'absolute',
    top: 0,
    bottom: 0,
    backgroundColor: '#3c87f755',
    borderColor: '#3c87f7',
    borderTopWidth: 3,
    borderBottomWidth: 3,
    minWidth: MIN_STATIC_GAP_PX,
  },
  handle: {
    position: 'absolute',
    top: 0,
    bottom: 0,
    width: HANDLE_WIDTH,
    borderRadius: Spacing.one,
    backgroundColor: '#3c87f7',
    borderWidth: 2,
    borderColor: '#ffffff',
  },
  playhead: {
    width: 4,
    marginLeft: HANDLE_WIDTH / 2 - 2,
    backgroundColor: '#ff3b30',
  },
  hint: {
    textAlign: 'center',
  },
});

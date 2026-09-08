import { useCallback } from 'react';
import { LayoutChangeEvent, StyleSheet } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, { runOnJS, useAnimatedStyle, useSharedValue } from 'react-native-reanimated';

import { CropRect } from '@/lib/api';

const MIN_SIDE_PX = 60;
const HANDLE_SIZE = 28;

function clamp(value: number, min: number, max: number): number {
  'worklet';
  return Math.min(Math.max(value, min), max);
}

/**
 * A square, draggable/resizable crop box over the video preview.
 *
 * Relies on the parent rendering the preview at the *exact* video aspect
 * ratio (see EditorScreen: `aspectRatio: metadata.width / metadata.height`)
 * — that's what makes a visually-square box in screen pixels correspond to
 * an equally-square region in the source video's own pixel space, so the
 * normalized rect this emits can be fed straight to the backend's
 * crop_to_pixels without distortion.
 */
export function CropOverlay({
  value,
  onChange,
  onContainerLayout,
}: {
  value: CropRect;
  onChange: (rect: CropRect) => void;
  /** Called once with the measured container size, in case the parent needs it. */
  onContainerLayout?: (width: number, height: number) => void;
}) {
  const containerWidth = useSharedValue(0);
  const containerHeight = useSharedValue(0);
  const boxX = useSharedValue(0);
  const boxY = useSharedValue(0);
  const boxSide = useSharedValue(0);
  const initialized = useSharedValue(false);

  const commit = useCallback(
    (x: number, y: number, side: number) => {
      const cw = containerWidth.value;
      const ch = containerHeight.value;
      if (cw <= 0 || ch <= 0) return;
      onChange({ x: x / cw, y: y / ch, width: side / cw, height: side / ch });
    },
    [onChange, containerWidth, containerHeight],
  );

  const onLayout = useCallback(
    (e: LayoutChangeEvent) => {
      const { width, height } = e.nativeEvent.layout;
      containerWidth.value = width;
      containerHeight.value = height;
      onContainerLayout?.(width, height);
      if (!initialized.value) {
        initialized.value = true;
        boxSide.value = clamp(value.width * width, MIN_SIDE_PX, Math.min(width, height));
        boxX.value = clamp(value.x * width, 0, width - boxSide.value);
        boxY.value = clamp(value.y * height, 0, height - boxSide.value);
      }
    },
    [containerWidth, containerHeight, initialized, boxSide, boxX, boxY, value, onContainerLayout],
  );

  const dragGesture = Gesture.Pan().onChange((e) => {
    boxX.value = clamp(boxX.value + e.changeX, 0, containerWidth.value - boxSide.value);
    boxY.value = clamp(boxY.value + e.changeY, 0, containerHeight.value - boxSide.value);
    runOnJS(commit)(boxX.value, boxY.value, boxSide.value);
  });

  const resizeGesture = Gesture.Pan().onChange((e) => {
    const maxSide = Math.min(containerWidth.value - boxX.value, containerHeight.value - boxY.value);
    boxSide.value = clamp(boxSide.value + (e.changeX + e.changeY) / 2, MIN_SIDE_PX, maxSide);
    runOnJS(commit)(boxX.value, boxY.value, boxSide.value);
  });

  const boxStyle = useAnimatedStyle(() => ({
    transform: [{ translateX: boxX.value }, { translateY: boxY.value }],
    width: boxSide.value,
    height: boxSide.value,
  }));
  const handleStyle = useAnimatedStyle(() => ({
    transform: [
      { translateX: boxX.value + boxSide.value - HANDLE_SIZE / 2 },
      { translateY: boxY.value + boxSide.value - HANDLE_SIZE / 2 },
    ],
  }));

  return (
    <>
      <GestureDetector gesture={dragGesture}>
        <Animated.View onLayout={onLayout} style={[StyleSheet.absoluteFill, styles.hitArea]}>
          <Animated.View style={[styles.box, boxStyle]} pointerEvents="none" />
        </Animated.View>
      </GestureDetector>
      <GestureDetector gesture={resizeGesture}>
        <Animated.View style={[styles.handle, handleStyle]} />
      </GestureDetector>
    </>
  );
}

const styles = StyleSheet.create({
  hitArea: {
    // Absolutely positioned to cover the whole preview so the drag gesture
    // can start from anywhere over the video, not just inside the box.
  },
  box: {
    position: 'absolute',
    borderWidth: 2,
    borderColor: '#ffffff',
    backgroundColor: '#ffffff22',
  },
  handle: {
    position: 'absolute',
    width: HANDLE_SIZE,
    height: HANDLE_SIZE,
    borderRadius: HANDLE_SIZE / 2,
    backgroundColor: '#3c87f7',
    borderWidth: 2,
    borderColor: '#ffffff',
  },
});

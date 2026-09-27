import { VideoPlayer, VideoView } from 'expo-video';
import { useCallback, useState } from 'react';
import { Image, LayoutChangeEvent, StyleSheet, View } from 'react-native';
import { Gesture, GestureDetector } from 'react-native-gesture-handler';
import Animated, { runOnJS, useAnimatedStyle, useSharedValue } from 'react-native-reanimated';

import { CropRect } from '@/lib/api';

// A small checkerboard tile, tiled with resizeMode="repeat" — the standard
// "this area is transparent" convention from image editors. Stands in for
// the black bars whenever the video doesn't fill the whole 1:1 frame,
// matching what the sticker will actually look like (transparent, not a
// solid black box) once generated. Generated as a 20x20 PNG, two 10x10
// light-gray/white quadrants per corner.
const CHECKERBOARD_TILE_URI =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABQAAAAUCAIAAAAC64paAAAALElEQVR4nGN8/fo1A25w9+5dPLJMeOQIglHNI0MzC/40pKysTCubRzWPDM0AnWgIglKmgMcAAAAASUVORK5CYII=';

// Zoom never goes below 1 — that's "whole video visible" (the letterboxed
// baseline). Zooming in beyond COVER_ZOOM_HEADROOM past full-cover (no more
// transparent bars) allows tighter framing, capped so extreme zoom doesn't
// ask the backend to upscale a tiny source region too far past its own
// detail.
const COVER_ZOOM_HEADROOM = 3;
const MAX_ZOOM_CAP = 8;

function clamp(value: number, min: number, max: number): number {
  'worklet';
  return Math.min(Math.max(value, min), max);
}

/**
 * Square, fixed viewport (matches the sticker's own 1:1 output) that the
 * video is pinch-zoomed and panned within — like Instagram's crop tool,
 * not a movable crop box over a fixed video. The default state (zoom=1)
 * shows the *entire* video letterboxed/pillarboxed against a checkerboard
 * (the standard "this is transparent" convention); pinch to zoom in and
 * fill the frame.
 *
 * Emits a `CropRect` that can legitimately extend outside [0,1] (negative
 * x/y, or x+width > 1) at low zoom — the backend pads that overflow with
 * transparency instead of clamping it away, so what's previewed here is
 * exactly what the sticker will look like (WYSIWYG): a proper transparent
 * cutout, not a black box.
 */
export function ZoomableVideoCrop({
  player,
  videoWidth,
  videoHeight,
  value,
  onChange,
}: {
  player: VideoPlayer;
  videoWidth: number;
  videoHeight: number;
  value: CropRect;
  onChange: (rect: CropRect) => void;
}) {
  const [viewportSize, setViewportSize] = useState(0);

  const videoAspect = videoWidth / videoHeight;
  // "Contain" fit at zoom=1: the video's larger-relative-to-square
  // dimension exactly matches the viewport; the other is proportionally
  // smaller, leaving letterbox/pillarbox space.
  const baseWidth = videoAspect >= 1 ? viewportSize : viewportSize * videoAspect;
  const baseHeight = videoAspect >= 1 ? viewportSize / videoAspect : viewportSize;
  const coverZoom = Math.max(videoAspect, 1 / videoAspect);
  const maxZoom = Math.min(MAX_ZOOM_CAP, coverZoom * COVER_ZOOM_HEADROOM);

  const zoom = useSharedValue(1);
  const panX = useSharedValue(0);
  const panY = useSharedValue(0);
  const initialized = useSharedValue(false);

  const commit = useCallback(
    (z: number, px: number, py: number) => {
      if (viewportSize <= 0 || baseWidth <= 0 || baseHeight <= 0) return;
      const dispW = baseWidth * z;
      const dispH = baseHeight * z;
      const left = (viewportSize - dispW) / 2 + px;
      const top = (viewportSize - dispH) / 2 + py;
      onChange({
        x: -left / dispW,
        y: -top / dispH,
        width: viewportSize / dispW,
        height: viewportSize / dispH,
      });
    },
    [onChange, viewportSize, baseWidth, baseHeight],
  );

  const onLayout = useCallback(
    (e: LayoutChangeEvent) => {
      const { width, height } = e.nativeEvent.layout;
      const side = Math.min(width, height);
      if (side <= 0) return;
      setViewportSize(side);
      if (!initialized.value) {
        initialized.value = true;
        // Reflect an incoming `value` (e.g. re-opening the editor) if it's
        // roughly the default full-frame crop; otherwise just start at the
        // zoomed-out baseline — reverse-engineering an arbitrary crop back
        // into zoom/pan isn't worth the complexity for what is always a
        // fresh editor session in this app today.
        commit(1, 0, 0);
      }
    },
    [initialized, commit],
  );

  const panGesture = Gesture.Pan().onChange((e) => {
    const dispW = baseWidth * zoom.value;
    const dispH = baseHeight * zoom.value;
    const maxPanX = Math.max(0, (dispW - viewportSize) / 2);
    const maxPanY = Math.max(0, (dispH - viewportSize) / 2);
    panX.value = clamp(panX.value + e.changeX, -maxPanX, maxPanX);
    panY.value = clamp(panY.value + e.changeY, -maxPanY, maxPanY);
    runOnJS(commit)(zoom.value, panX.value, panY.value);
  });

  const pinchGesture = Gesture.Pinch().onChange((e) => {
    zoom.value = clamp(zoom.value * e.scaleChange, 1, maxZoom);
    // Re-clamp pan for the new zoom level so it doesn't get stuck pointing
    // at now-invalid empty space after zooming back out.
    const dispW = baseWidth * zoom.value;
    const dispH = baseHeight * zoom.value;
    const maxPanX = Math.max(0, (dispW - viewportSize) / 2);
    const maxPanY = Math.max(0, (dispH - viewportSize) / 2);
    panX.value = clamp(panX.value, -maxPanX, maxPanX);
    panY.value = clamp(panY.value, -maxPanY, maxPanY);
    runOnJS(commit)(zoom.value, panX.value, panY.value);
  });

  const gesture = Gesture.Simultaneous(panGesture, pinchGesture);

  const videoStyle = useAnimatedStyle(() => ({
    width: baseWidth,
    height: baseHeight,
    transform: [{ translateX: panX.value }, { translateY: panY.value }, { scale: zoom.value }],
  }));

  return (
    <GestureDetector gesture={gesture}>
      <View style={styles.viewport} onLayout={onLayout}>
        <Image source={{ uri: CHECKERBOARD_TILE_URI }} style={StyleSheet.absoluteFill} resizeMode="repeat" />
        {viewportSize > 0 && (
          <Animated.View style={videoStyle}>
            <VideoView
              player={player}
              style={StyleSheet.absoluteFill}
              contentFit="fill"
              nativeControls={false}
            />
          </Animated.View>
        )}
      </View>
    </GestureDetector>
  );
}

const styles = StyleSheet.create({
  viewport: {
    width: '100%',
    height: '100%',
    overflow: 'hidden',
    alignItems: 'center',
    justifyContent: 'center',
  },
});

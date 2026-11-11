import { useEventListener } from "expo";
import { useVideoPlayer } from "expo-video";
import { router, useLocalSearchParams } from "expo-router";
import { StatusBar } from "expo-status-bar";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ActivityIndicator,
  LayoutChangeEvent,
  Pressable,
  StyleSheet,
  Text,
  View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import Animated, {
  Easing,
  useAnimatedStyle,
  useSharedValue,
  withRepeat,
  withSequence,
  withTiming,
} from "react-native-reanimated";

import { Kiosk, Spacing } from "@/constants/theme";
import { ZoomableVideoCrop } from "@/features/editor/ZoomableVideoCrop";
import { addSegment, Segment } from "@/features/editor/segments";
import { TrimTimeline } from "@/features/editor/TrimTimeline";
import {
  absoluteUrl,
  ApiRequestError,
  CropRect,
  StickerBackground,
} from "@/lib/api";
import { useCreateRender, useJob, useRender } from "@/lib/queries";

const SPEED_OPTIONS = [0.5, 0.75, 1, 1.2, 1.5, 2] as const;
const DEFAULT_ANIMATED_CLIP_SECONDS = 3;
// WhatsApp's own hard cap is 10s (see server/app/core/config.py), but past
// ~5s the 500KB budget forces the fitter deep into its fps/quality ladder
// and definition drops noticeably (see the earlier ladder-tuning work) — 5s
// is where quality still holds up well, so the editor doesn't offer more
// than that even though the backend would technically allow it.
const MAX_ANIMATED_CLIP_SECONDS = 5;

/** The "whole video visible, letterboxed" crop — same math as
 * ZoomableVideoCrop's zoom=1/pan=0 baseline, computed here too so the
 * initial `crop` state (used before that component's first layout fires)
 * already matches instead of flashing a different default. Can extend
 * outside [0,1] on purpose — see CropRect's docstring on the backend. */
function fullFrameCrop(width: number, height: number): CropRect {
  const aspect = width / height;
  if (aspect >= 1) {
    return { x: 0, y: -(aspect - 1) / 2, width: 1, height: aspect };
  }
  return {
    x: -(1 - aspect) / (2 * aspect),
    y: 0,
    width: 1 / aspect,
    height: 1,
  };
}

export default function EditorScreen() {
  const { jobId } = useLocalSearchParams<{ jobId: string }>();
  const { data: job } = useJob(jobId);
  const createRender = useCreateRender(jobId);

  const [mode, setMode] = useState<"animated" | "static">("animated");
  const [background, setBackground] = useState<StickerBackground>("blur");
  const [speed, setSpeed] = useState<number>(1);
  const [crop, setCrop] = useState<CropRect | null>(null);
  const [segments, setSegments] = useState<Segment[]>([
    { start: 0, end: DEFAULT_ANIMATED_CLIP_SECONDS },
  ]);
  const [activeSegment, setActiveSegment] = useState(0);
  const [frameAt, setFrameAt] = useState(0);
  const [error, setError] = useState<string | null>(null);
  // Set once the render is created, cleared once we navigate away or it
  // fails — its mere presence (while not ready/failed) is what keeps the
  // button showing a spinner for the *whole* generation, not just the
  // instant POST that kicks it off.
  const [pendingRenderId, setPendingRenderId] = useState<string | undefined>();
  const { data: pendingRender } = useRender(pendingRenderId);

  // The exact square (1:1) the sticker's 512x512 output maps to. Computed
  // from the measured flex space rather than an aspectRatio style — see the
  // comment at the JSX below for why.
  const [previewSize, setPreviewSize] = useState<{
    width: number;
    height: number;
  }>({
    width: 0,
    height: 0,
  });
  const onPreviewFlexLayout = useCallback((e: LayoutChangeEvent) => {
    const { width, height } = e.nativeEvent.layout;
    const side = Math.min(width, height);
    setPreviewSize({ width: side, height: side });
  }, []);

  const metadata = job?.metadata;

  useEffect(() => {
    if (!metadata || crop) return;
    setCrop(fullFrameCrop(metadata.width, metadata.height));
    const clipLength = Math.min(
      DEFAULT_ANIMATED_CLIP_SECONDS,
      metadata.duration,
    );
    setSegments([{ start: 0, end: clipLength }]);
    setActiveSegment(0);
    setFrameAt(metadata.duration / 2);
  }, [metadata, crop]);

  const proxyUrl = useMemo(
    () => absoluteUrl(job?.proxy_url ?? null),
    [job?.proxy_url],
  );
  const player = useVideoPlayer(proxyUrl ?? "", (p) => {
    p.loop = true;
    p.muted = true;
    p.timeUpdateEventInterval = 0.05; // default is 0 (event never fires) — needed to hop between segments below
  });

  // Kept in refs (not state) so the timeUpdate listener below always reads
  // the latest values without needing to resubscribe on every keystroke of
  // dragging a trim handle.
  const modeRef = useRef(mode);
  modeRef.current = mode;
  const segmentsRef = useRef(segments);
  segmentsRef.current = segments;
  const speedRef = useRef(speed);
  speedRef.current = speed;

  // Live playback position for the moving line on the timeline. A shared
  // value (not React state) so it updates on the UI thread ~10x/sec without
  // re-rendering this whole screen every tick.
  const playbackTime = useSharedValue(0);

  // Preview only the selected pieces: whenever playback leaves a segment
  // (or the user just dragged one so the current position now falls
  // outside), jump to the start of the next segment — wrapping to the first
  // after the last — instead of playing the whole video.
  useEventListener(player, "timeUpdate", ({ currentTime }) => {
    playbackTime.value = currentTime;
    if (modeRef.current !== "animated") return;
    const segs = segmentsRef.current;
    // Jump one event early (in video seconds, so scaled by playback speed):
    // the event only fires every 50ms, and waiting until we're past `end`
    // would show frames of the gap that aren't in the sticker.
    const lookahead = 0.05 * speedRef.current;
    if (
      segs.some(
        (sg) => currentTime >= sg.start && currentTime < sg.end - lookahead,
      )
    )
      return;
    player.currentTime = (
      segs.find((sg) => sg.start >= currentTime) ?? segs[0]
    ).start;
  });

  // Switching modes: animated plays (looping within [start, end] via the
  // listener above); static pauses on a single frame instead.
  useEffect(() => {
    if (!proxyUrl) return;
    if (mode === "static") {
      player.pause();
    } else {
      player.currentTime = segmentsRef.current[0].start;
      player.play();
    }
    // Deliberately not depending on segments/frameAt — those are handled
    // continuously by the timeUpdate listener / the effect below instead,
    // so dragging a handle doesn't restart playback on every frame.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [proxyUrl, mode, player]);

  // The preview plays at the chosen speed, same as the generated sticker.
  useEffect(() => {
    player.playbackRate = speed;
  }, [player, speed]);

  // Static mode: keep the shown frame in sync while dragging the playhead.
  useEffect(() => {
    if (mode === "static" && proxyUrl) {
      player.currentTime = frameAt;
    }
  }, [mode, proxyUrl, frameAt, player]);

  // Only leave this screen once the sticker is actually done — the button
  // stays in its loading state the whole time (see isGenerating below),
  // instead of the spinner clearing right after the near-instant POST
  // while the real work still happens invisibly on the next screen.
  useEffect(() => {
    if (!pendingRender) return;
    if (pendingRender.status === "ready") {
      router.replace({
        pathname: "/preview",
        params: { renderId: pendingRender.render_id },
      });
    } else if (pendingRender.status === "failed") {
      setError(
        pendingRender.error?.message ?? "No se pudo generar el sticker.",
      );
      setPendingRenderId(undefined);
    }
  }, [pendingRender]);

  const isGenerating =
    createRender.isPending ||
    (!!pendingRenderId &&
      pendingRender?.status !== "ready" &&
      pendingRender?.status !== "failed");

  // The kiosk sign "warming up" — same motif as Continuar/Añadir a WhatsApp.
  const pulse = useSharedValue(1);
  useEffect(() => {
    if (isGenerating) {
      pulse.value = withRepeat(
        withSequence(
          withTiming(0.5, { duration: 550, easing: Easing.out(Easing.quad) }),
          withTiming(1, { duration: 550, easing: Easing.out(Easing.quad) }),
        ),
        -1,
      );
    } else {
      pulse.value = withTiming(1, { duration: 200 });
    }
  }, [isGenerating, pulse]);
  const pulseStyle = useAnimatedStyle(() => ({ opacity: pulse.value }));

  if (!job || job.status === "failed") {
    return (
      <View style={[styles.container, styles.center]}>
        <StatusBar style="light" />
        <Text style={styles.errorText}>
          {job?.error?.message ?? "No se pudo cargar el video."}
        </Text>
      </View>
    );
  }

  if (job.status !== "ready" || !metadata || !crop) {
    return (
      <View style={[styles.container, styles.center]}>
        <StatusBar style="light" />
        <ActivityIndicator color={Kiosk.accent} />
        <Text style={styles.statusText}>
          {job.status === "downloading"
            ? "Descargando video…"
            : "Procesando video…"}
        </Text>
      </View>
    );
  }

  // One budget shared by all pieces together (see MAX_ANIMATED_CLIP_SECONDS).
  const maxTotalSeconds = Math.min(
    MAX_ANIMATED_CLIP_SECONDS,
    metadata.duration,
  );
  const added = addSegment(segments, metadata.duration, maxTotalSeconds);

  const handleAddSegment = () => {
    if (!added) return;
    setSegments(added.segments);
    setActiveSegment(added.index);
  };

  const handleRemoveSegment = () => {
    if (segments.length < 2) return;
    setSegments(segments.filter((_, i) => i !== activeSegment));
    setActiveSegment(Math.max(0, activeSegment - 1));
  };

  const handleGenerate = async () => {
    setError(null);
    try {
      const render = await createRender.mutateAsync(
        mode === "animated"
          ? {
              type: "animated",
              start: segments[0].start,
              end: segments[segments.length - 1].end,
              segments,
              crop,
              emojis: [],
              background,
              speed,
            }
          : {
              type: "static",
              start: frameAt,
              frame_at: frameAt,
              crop,
              emojis: [],
              background,
            },
      );
      setPendingRenderId(render.render_id);
    } catch (e) {
      setError(
        e instanceof ApiRequestError
          ? e.message
          : "No se pudo generar el sticker.",
      );
    }
  };

  return (
    <View style={styles.container}>
      <StatusBar style="light" />
      <SafeAreaView style={styles.safeArea} edges={["bottom"]}>
        {/* flex:1 so this only gets the space left over after `controls`
            below claims its natural height — otherwise a preview taking
            the full available height could push the timeline/button off
            the bottom of the screen. */}
        <View style={styles.modeToggle}>
          {(
            [
              ["transparent", "Transparente"],
              ["blur", "Difuminado"],
            ] as const
          ).map(([value, label]) => (
            <Pressable
              key={value}
              onPress={() => setBackground(value)}
              style={[
                styles.modeButton,
                background === value && styles.modeButtonActive,
              ]}>
              <Text
                style={[
                  styles.modeButtonLabel,
                  background === value && styles.modeButtonLabelActive,
                ]}>
                {label}
              </Text>
            </Pressable>
          ))}
        </View>

        <View style={styles.previewFlex} onLayout={onPreviewFlexLayout}>
          {/* Always an exact square (1:1) — matches the sticker's own
              512x512 output, so this frame's edges are precisely the crop
              boundary: what's inside (black bars included) is what you get.
              Sized in JS from the measured flex space rather than
              aspectRatio+percentage styles, which didn't reliably resolve
              to a square nested in a flex:1 parent. */}
          <View style={[styles.previewContainer, previewSize]}>
            <ZoomableVideoCrop
              player={player}
              videoWidth={metadata.width}
              videoHeight={metadata.height}
              value={crop}
              onChange={setCrop}
            />
          </View>
        </View>

        <View style={styles.controls}>
          <View style={styles.modeToggle}>
            {(["animated", "static"] as const).map((m) => (
              <Pressable
                key={m}
                onPress={() => setMode(m)}
                style={[
                  styles.modeButton,
                  mode === m && styles.modeButtonActive,
                ]}>
                <Text
                  style={[
                    styles.modeButtonLabel,
                    mode === m && styles.modeButtonLabelActive,
                  ]}>
                  {m === "animated" ? "Animado" : "Foto"}
                </Text>
              </Pressable>
            ))}
          </View>

          {mode === "animated" && (
            <View style={styles.modeToggle}>
              {SPEED_OPTIONS.map((s) => (
                <Pressable
                  key={s}
                  onPress={() => setSpeed(s)}
                  style={[
                    styles.modeButton,
                    speed === s && styles.modeButtonActive,
                  ]}>
                  <Text
                    style={[
                      styles.modeButtonLabel,
                      speed === s && styles.modeButtonLabelActive,
                    ]}>
                    {`x${s}`}
                  </Text>
                </Pressable>
              ))}
            </View>
          )}

          {mode === "animated" ? (
            <>
            <TrimTimeline
              mode="animated"
              speed={speed}
              durationSec={metadata.duration}
              spriteUri={absoluteUrl(job.sprite_url)}
              segments={segments}
              activeIndex={activeSegment}
              onSelect={setActiveSegment}
              maxTotalSeconds={maxTotalSeconds}
              onChange={(s, e) =>
                setSegments((prev) =>
                  prev.map((sg, i) =>
                    i === activeSegment ? { start: s, end: e } : sg,
                  ),
                )
              }
              playbackTime={playbackTime}
              onDragEnd={() => {
                player.currentTime = segmentsRef.current[activeSegment].start;
                player.play();
              }}
            />
            <View style={styles.segmentActions}>
              <Pressable
                onPress={handleAddSegment}
                disabled={!added}
                style={[
                  styles.segmentButton,
                  !added && styles.segmentButtonDisabled,
                ]}>
                <Text style={styles.segmentButtonLabel}>+ Intervalo</Text>
              </Pressable>
              {segments.length > 1 && (
                <Pressable
                  onPress={handleRemoveSegment}
                  style={styles.segmentButton}>
                  <Text style={styles.segmentButtonLabel}>Quitar este</Text>
                </Pressable>
              )}
            </View>
            </>
          ) : (
            <TrimTimeline
              mode="static"
              durationSec={metadata.duration}
              spriteUri={absoluteUrl(job.sprite_url)}
              frameAt={frameAt}
              onChange={setFrameAt}
            />
          )}

          {error && <Text style={styles.errorText}>{error}</Text>}

          <Animated.View style={pulseStyle}>
            <Pressable
              onPress={handleGenerate}
              disabled={isGenerating}
              style={({ pressed }) => [
                styles.generateButton,
                { opacity: pressed ? 0.85 : 1 },
              ]}>
              <Text style={styles.generateButtonLabel}>
                {isGenerating ? "Generando sticker…" : "Generar sticker"}
              </Text>
            </Pressable>
          </Animated.View>
        </View>
      </SafeAreaView>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: Kiosk.background },
  safeArea: { flex: 1, padding: Spacing.three, gap: Spacing.three },
  center: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    gap: Spacing.two,
    padding: Spacing.four,
  },
  statusText: { textAlign: "center", fontSize: 14, color: Kiosk.textSecondary },
  previewFlex: {
    flex: 1,
    minHeight: 0, // lets this actually shrink instead of overflowing its flex share
    alignItems: "center",
    justifyContent: "center",
  },
  previewContainer: {
    // width/height set inline from measured previewSize — see above.
    // No backgroundColor here: ZoomableVideoCrop fills this with its own
    // checkerboard (transparency) pattern.
    borderRadius: Spacing.two,
    borderWidth: 2,
    borderColor: Kiosk.accent,
    overflow: "hidden",
  },
  controls: {
    gap: Spacing.three,
  },
  modeToggle: {
    flexDirection: "row",
    borderRadius: Spacing.three,
    padding: 4,
    backgroundColor: Kiosk.inset,
    borderWidth: 1,
    borderColor: Kiosk.border,
    alignSelf: "center",
  },
  segmentActions: {
    flexDirection: "row",
    justifyContent: "center",
    gap: Spacing.two,
  },
  segmentButton: {
    paddingVertical: Spacing.two,
    paddingHorizontal: Spacing.three,
    borderRadius: Spacing.two + Spacing.one,
    backgroundColor: Kiosk.inset,
    borderWidth: 1,
    borderColor: Kiosk.border,
  },
  segmentButtonDisabled: {
    opacity: 0.4,
  },
  segmentButtonLabel: {
    fontSize: 14,
    fontWeight: "700",
    color: Kiosk.textSecondary,
  },
  modeButton: {
    paddingVertical: Spacing.two,
    paddingHorizontal: Spacing.four,
    borderRadius: Spacing.two + Spacing.one,
  },
  modeButtonActive: {
    backgroundColor: Kiosk.accent,
  },
  modeButtonLabel: {
    fontSize: 14,
    fontWeight: "700",
    color: Kiosk.textSecondary,
  },
  modeButtonLabelActive: {
    color: Kiosk.onAccent,
  },
  generateButton: {
    paddingVertical: Spacing.three,
    borderRadius: 14,
    alignItems: "center",
    backgroundColor: Kiosk.accent,
  },
  generateButtonLabel: {
    fontSize: 15,
    fontWeight: "700",
    letterSpacing: 0.2,
    color: Kiosk.onAccent,
  },
  errorText: {
    fontSize: 13,
    color: Kiosk.error,
    textAlign: "center",
  },
});

import { useVideoPlayer, VideoView } from 'expo-video';
import { router, useLocalSearchParams } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { ThemedText } from '@/components/themed-text';
import { ThemedView } from '@/components/themed-view';
import { Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';
import { CropOverlay } from '@/features/editor/CropOverlay';
import { TrimTimeline } from '@/features/editor/TrimTimeline';
import { absoluteUrl, ApiRequestError, CropRect } from '@/lib/api';
import { useCreateRender, useJob } from '@/lib/queries';

const DEFAULT_ANIMATED_CLIP_SECONDS = 3;
const MAX_ANIMATED_CLIP_SECONDS = 10; // WhatsApp's hard cap — see server/app/core/config.py

function centeredSquareCrop(width: number, height: number): CropRect {
  const side = Math.min(width, height) / Math.max(width, height);
  return width >= height
    ? { x: (1 - side) / 2, y: 0, width: side, height: 1 }
    : { x: 0, y: (1 - side) / 2, width: 1, height: side };
}

export default function EditorScreen() {
  const { jobId } = useLocalSearchParams<{ jobId: string }>();
  const { data: job } = useJob(jobId);
  const theme = useTheme();
  const createRender = useCreateRender(jobId);

  const [mode, setMode] = useState<'animated' | 'static'>('animated');
  const [crop, setCrop] = useState<CropRect | null>(null);
  const [start, setStart] = useState(0);
  const [end, setEnd] = useState(DEFAULT_ANIMATED_CLIP_SECONDS);
  const [frameAt, setFrameAt] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const metadata = job?.metadata;

  useEffect(() => {
    if (!metadata || crop) return;
    setCrop(centeredSquareCrop(metadata.width, metadata.height));
    const clipLength = Math.min(DEFAULT_ANIMATED_CLIP_SECONDS, metadata.duration);
    setStart(0);
    setEnd(clipLength);
    setFrameAt(metadata.duration / 2);
  }, [metadata, crop]);

  const proxyUrl = useMemo(() => absoluteUrl(job?.proxy_url ?? null), [job?.proxy_url]);
  const player = useVideoPlayer(proxyUrl ?? '', (p) => {
    p.loop = true;
    p.muted = true;
  });

  useEffect(() => {
    if (proxyUrl) player.play();
  }, [proxyUrl, player]);

  if (!job || job.status === 'failed') {
    return (
      <ThemedView style={styles.center}>
        <ThemedText type="small">
          {job?.error?.message ?? 'No se pudo cargar el video.'}
        </ThemedText>
      </ThemedView>
    );
  }

  if (job.status !== 'ready' || !metadata || !crop) {
    return (
      <ThemedView style={styles.center}>
        <ActivityIndicator />
        <ThemedText type="small" themeColor="textSecondary" style={styles.statusText}>
          {job.status === 'downloading' ? 'Descargando video…' : 'Procesando video…'}
        </ThemedText>
      </ThemedView>
    );
  }

  const handleGenerate = async () => {
    setError(null);
    try {
      const render = await createRender.mutateAsync(
        mode === 'animated'
          ? { type: 'animated', start, end, crop, emojis: [] }
          : { type: 'static', start: frameAt, frame_at: frameAt, crop, emojis: [] },
      );
      router.replace({ pathname: '/preview', params: { renderId: render.render_id } });
    } catch (e) {
      setError(e instanceof ApiRequestError ? e.message : 'No se pudo generar el sticker.');
    }
  };

  return (
    <ThemedView style={styles.container}>
      <SafeAreaView style={styles.safeArea} edges={['bottom']}>
        <View style={[styles.previewContainer, { aspectRatio: metadata.width / metadata.height }]}>
          <VideoView player={player} style={StyleSheet.absoluteFill} contentFit="cover" nativeControls={false} />
          <CropOverlay value={crop} onChange={setCrop} />
        </View>

        <ThemedView style={styles.modeToggle}>
          {(['animated', 'static'] as const).map((m) => (
            <Pressable
              key={m}
              onPress={() => setMode(m)}
              style={[
                styles.modeButton,
                { backgroundColor: mode === m ? theme.text : 'transparent' },
              ]}>
              <ThemedText type="smallBold" themeColor={mode === m ? 'background' : 'text'}>
                {m === 'animated' ? 'Animado' : 'Foto'}
              </ThemedText>
            </Pressable>
          ))}
        </ThemedView>

        {mode === 'animated' ? (
          <TrimTimeline
            mode="animated"
            durationSec={metadata.duration}
            spriteUri={absoluteUrl(job.sprite_url)}
            start={start}
            end={end}
            maxClipSeconds={Math.min(MAX_ANIMATED_CLIP_SECONDS, metadata.duration)}
            onChange={(s, e) => {
              setStart(s);
              setEnd(e);
            }}
          />
        ) : (
          <TrimTimeline
            mode="static"
            durationSec={metadata.duration}
            spriteUri={absoluteUrl(job.sprite_url)}
            frameAt={frameAt}
            onChange={setFrameAt}
          />
        )}

        {error && (
          <ThemedText type="small" style={styles.error}>
            {error}
          </ThemedText>
        )}

        <Pressable
          onPress={handleGenerate}
          disabled={createRender.isPending}
          style={({ pressed }) => [
            styles.generateButton,
            { backgroundColor: theme.text, opacity: pressed || createRender.isPending ? 0.7 : 1 },
          ]}>
          {createRender.isPending ? (
            <ActivityIndicator color={theme.background} />
          ) : (
            <ThemedText type="smallBold" themeColor="background">
              Generar sticker
            </ThemedText>
          )}
        </Pressable>
      </SafeAreaView>
    </ThemedView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  safeArea: { flex: 1, padding: Spacing.three, gap: Spacing.three },
  center: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: Spacing.two, padding: Spacing.four },
  statusText: { textAlign: 'center' },
  previewContainer: {
    width: '100%',
    borderRadius: Spacing.three,
    overflow: 'hidden',
    backgroundColor: '#000',
  },
  modeToggle: {
    flexDirection: 'row',
    borderRadius: Spacing.three,
    padding: 4,
    backgroundColor: '#00000011',
    alignSelf: 'center',
  },
  modeButton: {
    paddingVertical: Spacing.two,
    paddingHorizontal: Spacing.four,
    borderRadius: Spacing.two,
  },
  generateButton: {
    paddingVertical: Spacing.three,
    borderRadius: Spacing.three,
    alignItems: 'center',
  },
  error: {
    color: '#ff3b30',
    textAlign: 'center',
  },
});

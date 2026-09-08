import * as Clipboard from 'expo-clipboard';
import { router } from 'expo-router';
import { useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Platform,
  Pressable,
  StyleSheet,
  TextInput,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { ThemedText } from '@/components/themed-text';
import { ThemedView } from '@/components/themed-view';
import { BottomTabInset, MaxContentWidth, Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';
import { ApiRequestError } from '@/lib/api';
import { useCreateJob } from '@/lib/queries';

const TIKTOK_URL_PATTERN = /tiktok\.com/i;

export default function HomeScreen() {
  const [url, setUrl] = useState('');
  const [error, setError] = useState<string | null>(null);
  const theme = useTheme();
  const createJob = useCreateJob();

  // Quality-of-life: if the user copied a TikTok link right before opening
  // the app (the common flow — share sheet doesn't always work, copy does),
  // offer to paste it automatically.
  useEffect(() => {
    (async () => {
      try {
        const clipboard = await Clipboard.getStringAsync();
        if (clipboard && TIKTOK_URL_PATTERN.test(clipboard)) {
          setUrl(clipboard.trim());
        }
      } catch {
        // Clipboard access can fail/be denied silently — not worth surfacing.
      }
    })();
  }, []);

  const handleSubmit = async () => {
    setError(null);
    const trimmed = url.trim();
    if (!TIKTOK_URL_PATTERN.test(trimmed)) {
      setError('Pegá un link de TikTok válido.');
      return;
    }
    try {
      const job = await createJob.mutateAsync(trimmed);
      router.push({ pathname: '/editor', params: { jobId: job.job_id } });
    } catch (e) {
      setError(e instanceof ApiRequestError ? e.message : 'No se pudo crear el job.');
    }
  };

  return (
    <ThemedView style={styles.container}>
      <SafeAreaView style={styles.safeArea}>
        <ThemedView style={styles.heroSection}>
          <ThemedText type="title" style={styles.title}>
            Stickers desde TikTok
          </ThemedText>
          <ThemedText type="small" themeColor="textSecondary" style={styles.subtitle}>
            Pegá el link de un video, recortalo y generá un sticker animado o de foto para
            WhatsApp.
          </ThemedText>
        </ThemedView>

        <ThemedView type="backgroundElement" style={styles.card}>
          <TextInput
            value={url}
            onChangeText={setUrl}
            placeholder="https://www.tiktok.com/@usuario/video/..."
            placeholderTextColor={theme.textSecondary}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
            style={[styles.input, { color: theme.text, borderColor: theme.backgroundSelected }]}
          />
          {error && (
            <ThemedText type="small" style={styles.error}>
              {error}
            </ThemedText>
          )}
          <Pressable
            onPress={handleSubmit}
            disabled={createJob.isPending || !url.trim()}
            style={({ pressed }) => [
              styles.button,
              { backgroundColor: theme.text, opacity: pressed || createJob.isPending ? 0.7 : 1 },
            ]}>
            {createJob.isPending ? (
              <ActivityIndicator color={theme.background} />
            ) : (
              <ThemedText type="smallBold" themeColor="background">
                Continuar
              </ThemedText>
            )}
          </Pressable>
        </ThemedView>
      </SafeAreaView>
    </ThemedView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: 'center',
    flexDirection: 'row',
  },
  safeArea: {
    flex: 1,
    paddingHorizontal: Spacing.four,
    justifyContent: 'center',
    gap: Spacing.five,
    paddingBottom: Platform.select({ android: BottomTabInset, default: 0 }) + Spacing.three,
    maxWidth: MaxContentWidth,
    width: '100%',
  },
  heroSection: {
    gap: Spacing.two,
  },
  title: {
    fontSize: 32,
    lineHeight: 38,
  },
  subtitle: {
    lineHeight: 20,
  },
  card: {
    borderRadius: Spacing.four,
    padding: Spacing.four,
    gap: Spacing.three,
  },
  input: {
    borderWidth: 1,
    borderRadius: Spacing.three,
    paddingHorizontal: Spacing.three,
    paddingVertical: Spacing.two,
    fontSize: 16,
  },
  button: {
    paddingVertical: Spacing.three,
    borderRadius: Spacing.three,
    alignItems: 'center',
    justifyContent: 'center',
  },
  error: {
    color: '#ff3b30',
  },
});

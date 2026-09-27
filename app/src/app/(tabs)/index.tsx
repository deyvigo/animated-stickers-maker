import * as Clipboard from 'expo-clipboard';
import { router } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useState } from 'react';
import { Platform, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import Animated, {
  Easing,
  useAnimatedStyle,
  useSharedValue,
  withRepeat,
  withSequence,
  withTiming,
} from 'react-native-reanimated';
import { SafeAreaView } from 'react-native-safe-area-context';

import { BottomTabInset, Kiosk, MaxContentWidth, Spacing } from '@/constants/theme';
import { ApiRequestError } from '@/lib/api';
import { useCreateJob } from '@/lib/queries';

const TIKTOK_URL_PATTERN = /tiktok\.com/i;

export default function HomeScreen() {
  const [url, setUrl] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [focused, setFocused] = useState(false);
  const createJob = useCreateJob();
  const pulse = useSharedValue(1);

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

  // The kiosk sign "warming up": while the job is created, the button
  // breathes instead of swapping in a generic spinner.
  useEffect(() => {
    if (createJob.isPending) {
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
  }, [createJob.isPending, pulse]);

  const pulseStyle = useAnimatedStyle(() => ({ opacity: pulse.value }));

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

  const idleDisabled = !createJob.isPending && !url.trim();

  return (
    <View style={styles.container}>
      <StatusBar style="light" />
      <SafeAreaView style={styles.safeArea}>
        <View style={styles.heroSection}>
          <Text style={styles.title}>Stickers desde TikTok</Text>
          <Text style={styles.subtitle}>
            Pegá el link de un video, recortalo y generá un sticker animado o de foto para
            WhatsApp.
          </Text>
        </View>

        <View style={styles.card}>
          <TextInput
            value={url}
            onChangeText={setUrl}
            onFocus={() => setFocused(true)}
            onBlur={() => setFocused(false)}
            placeholder="https://www.tiktok.com/@usuario/video/..."
            placeholderTextColor={Kiosk.textSecondary}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
            style={[styles.input, focused && styles.inputFocused]}
          />
          {error && <Text style={styles.error}>{error}</Text>}
          <Animated.View style={pulseStyle}>
            <Pressable
              onPress={handleSubmit}
              disabled={createJob.isPending || !url.trim()}
              style={({ pressed }) => [
                styles.button,
                { opacity: idleDisabled ? 0.4 : pressed ? 0.85 : 1 },
              ]}>
              <Text style={styles.buttonLabel}>
                {createJob.isPending ? 'Conectando…' : 'Continuar'}
              </Text>
            </Pressable>
          </Animated.View>
        </View>
      </SafeAreaView>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: Kiosk.background,
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
    fontSize: 34,
    lineHeight: 38,
    fontWeight: '700',
    letterSpacing: -0.5,
    color: Kiosk.text,
  },
  subtitle: {
    fontSize: 15,
    lineHeight: 21,
    fontWeight: '500',
    color: Kiosk.textSecondary,
  },
  card: {
    backgroundColor: Kiosk.surface,
    borderWidth: 1,
    borderColor: Kiosk.border,
    borderRadius: 20,
    padding: Spacing.four,
    gap: Spacing.three,
  },
  input: {
    borderWidth: 1,
    borderColor: Kiosk.border,
    borderRadius: 12,
    paddingHorizontal: Spacing.three,
    paddingVertical: Spacing.three,
    backgroundColor: Kiosk.inset,
    fontSize: 15,
    color: Kiosk.text,
  },
  inputFocused: {
    borderColor: Kiosk.borderFocused,
  },
  button: {
    paddingVertical: Spacing.three,
    borderRadius: 14,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: Kiosk.accent,
  },
  buttonLabel: {
    fontSize: 15,
    fontWeight: '700',
    letterSpacing: 0.2,
    color: Kiosk.onAccent,
  },
  error: {
    fontSize: 13,
    color: Kiosk.error,
  },
});

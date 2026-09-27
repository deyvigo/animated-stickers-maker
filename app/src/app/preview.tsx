import { Image } from 'expo-image';
import { router, useLocalSearchParams } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useState } from 'react';
import { ActivityIndicator, Alert, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import Animated, {
  Easing,
  useAnimatedStyle,
  useSharedValue,
  withRepeat,
  withSequence,
  withTiming,
} from 'react-native-reanimated';

import { Kiosk, Spacing } from '@/constants/theme';
import { useAddStickerToPack, useCreatePack, usePacks } from '@/features/packs/usePacks';
import { WHATSAPP_MAX_EMOJIS_PER_STICKER } from '@/features/packs/types';
import { absoluteUrl } from '@/lib/api';
import { useRender } from '@/lib/queries';

const QUICK_EMOJIS = ['😂', '😍', '🔥', '😢', '😱', '👍', '❤️', '😎', '🤣', '😡', '🎉', '👀'];

export default function PreviewScreen() {
  const { renderId } = useLocalSearchParams<{ renderId: string }>();
  const { data: render } = useRender(renderId);
  const { data: packs = [] } = usePacks();
  const createPack = useCreatePack();
  const addSticker = useAddStickerToPack();

  const [emojis, setEmojis] = useState<string[]>([]);
  const [newPackName, setNewPackName] = useState('');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  // The kiosk sign "warming up" — same motif as every other primary action.
  const pulse = useSharedValue(1);
  useEffect(() => {
    if (saving) {
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
  }, [saving, pulse]);
  const pulseStyle = useAnimatedStyle(() => ({ opacity: pulse.value }));

  if (!render || render.status === 'failed') {
    return (
      <View style={[styles.container, styles.center]}>
        <StatusBar style="light" />
        <Text style={styles.errorText}>{render?.error?.message ?? 'No se pudo generar el sticker.'}</Text>
        <Pressable onPress={() => router.back()} style={styles.retryButton}>
          <Text style={styles.retryButtonLabel}>Volver a intentar</Text>
        </Pressable>
      </View>
    );
  }

  if (render.status !== 'ready') {
    return (
      <View style={[styles.container, styles.center]}>
        <StatusBar style="light" />
        <ActivityIndicator color={Kiosk.accent} />
        <Text style={styles.statusText}>Generando sticker…</Text>
      </View>
    );
  }

  const toggleEmoji = (emoji: string) => {
    setEmojis((prev) => {
      if (prev.includes(emoji)) return prev.filter((e) => e !== emoji);
      if (prev.length >= WHATSAPP_MAX_EMOJIS_PER_STICKER) return prev;
      return [...prev, emoji];
    });
  };

  const compatiblePacks = packs.filter((p) => p.animatedStickerPack === (render.type === 'animated'));

  const saveToPack = async (packId: string) => {
    if (!render.sticker_url || !render.tray_url) return;
    setSaving(true);
    try {
      await addSticker.mutateAsync({
        packId,
        stickerUrl: render.sticker_url,
        trayUrl: render.tray_url,
        emojis,
      });
      setSaved(true);
    } catch (e) {
      Alert.alert('No se pudo guardar', e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const createAndSave = async () => {
    if (!newPackName.trim()) {
      Alert.alert('Poné un nombre para el pack');
      return;
    }
    const pack = await createPack.mutateAsync({
      name: newPackName,
      publisher: 'Yo',
      animated: render.type === 'animated',
    });
    setNewPackName('');
    await saveToPack(pack.identifier);
  };

  return (
    <View style={styles.container}>
      <StatusBar style="light" />
      <SafeAreaView style={styles.safeArea} edges={['bottom']}>
        <View style={styles.previewBox}>
          {render.sticker_url && (
            <Image source={{ uri: absoluteUrl(render.sticker_url) ?? undefined }} style={styles.sticker} contentFit="contain" />
          )}
        </View>

        <Text style={styles.centerText}>
          {((render.bytes ?? 0) / 1024).toFixed(0)} KB
          {render.fit?.degraded ? ' · calidad reducida para cumplir el límite de WhatsApp' : ''}
        </Text>

        {saved ? (
          <View style={styles.savedCard}>
            <Text style={styles.savedText}>¡Guardado en el pack!</Text>
            <Pressable onPress={() => router.dismissTo('/packs')}>
              <Text style={styles.savedLink}>Ir a Mis packs</Text>
            </Pressable>
          </View>
        ) : (
          <>
            <Text style={styles.label}>Emojis (hasta 3)</Text>
            <View style={styles.emojiRow}>
              {QUICK_EMOJIS.map((emoji) => (
                <Pressable
                  key={emoji}
                  onPress={() => toggleEmoji(emoji)}
                  style={[styles.emojiButton, emojis.includes(emoji) && styles.emojiButtonSelected]}>
                  <Text style={styles.emojiText}>{emoji}</Text>
                </Pressable>
              ))}
            </View>

            <Text style={styles.label}>Guardar en un pack</Text>
            {compatiblePacks.map((pack) => (
              <Pressable
                key={pack.identifier}
                onPress={() => saveToPack(pack.identifier)}
                disabled={saving || pack.stickers.length >= 30}
                style={({ pressed }) => [
                  styles.packRow,
                  { opacity: pressed || saving ? 0.7 : 1 },
                ]}>
                <Text style={styles.packRowName}>{pack.name}</Text>
                <Text style={styles.packRowCount}>{pack.stickers.length}/30</Text>
              </Pressable>
            ))}

            <View style={styles.newPackRow}>
              <TextInput
                value={newPackName}
                onChangeText={setNewPackName}
                placeholder="Nombre de un pack nuevo"
                placeholderTextColor={Kiosk.textSecondary}
                style={styles.input}
              />
              <Animated.View style={pulseStyle}>
                <Pressable
                  onPress={createAndSave}
                  disabled={saving}
                  style={({ pressed }) => [styles.createButton, { opacity: pressed ? 0.85 : 1 }]}>
                  <Text style={styles.createButtonLabel}>Crear</Text>
                </Pressable>
              </Animated.View>
            </View>
          </>
        )}
      </SafeAreaView>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: Kiosk.background },
  safeArea: { flex: 1, padding: Spacing.three, gap: Spacing.three },
  center: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: Spacing.two, padding: Spacing.four },
  centerText: { textAlign: 'center', fontSize: 13, color: Kiosk.textSecondary },
  statusText: { fontSize: 14, color: Kiosk.textSecondary },
  errorText: { fontSize: 14, color: Kiosk.error, textAlign: 'center' },
  previewBox: {
    aspectRatio: 1,
    borderRadius: Spacing.three,
    overflow: 'hidden',
    backgroundColor: Kiosk.surface,
    borderWidth: 1,
    borderColor: Kiosk.border,
    alignItems: 'center',
    justifyContent: 'center',
  },
  sticker: { width: '80%', height: '80%' },
  label: { fontSize: 14, fontWeight: '600', color: Kiosk.text },
  emojiRow: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.one },
  emojiButton: { padding: Spacing.one, borderRadius: Spacing.two },
  emojiButtonSelected: { backgroundColor: Kiosk.accent },
  emojiText: { fontSize: 28, lineHeight: 34 },
  packRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    paddingVertical: Spacing.two,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: Kiosk.border,
  },
  packRowName: { fontSize: 15, color: Kiosk.text },
  packRowCount: { fontSize: 13, color: Kiosk.textSecondary },
  newPackRow: { flexDirection: 'row', gap: Spacing.two, marginTop: Spacing.one },
  input: {
    flex: 1,
    borderWidth: 1,
    borderColor: Kiosk.border,
    borderRadius: 12,
    paddingHorizontal: Spacing.three,
    paddingVertical: Spacing.two,
    backgroundColor: Kiosk.inset,
    fontSize: 15,
    color: Kiosk.text,
  },
  createButton: {
    paddingHorizontal: Spacing.four,
    paddingVertical: Spacing.two,
    borderRadius: 12,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: Kiosk.accent,
  },
  createButtonLabel: { fontSize: 14, fontWeight: '700', color: Kiosk.onAccent },
  savedCard: {
    padding: Spacing.four,
    borderRadius: 20,
    gap: Spacing.two,
    alignItems: 'center',
    backgroundColor: Kiosk.surface,
    borderWidth: 1,
    borderColor: Kiosk.border,
  },
  savedText: { fontSize: 15, fontWeight: '700', color: Kiosk.text },
  savedLink: { fontSize: 14, fontWeight: '700', color: Kiosk.accent },
  retryButton: { marginTop: Spacing.two },
  retryButtonLabel: { fontSize: 14, fontWeight: '600', color: Kiosk.accent },
});

import { Image } from 'expo-image';
import { router, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { ActivityIndicator, Alert, Pressable, StyleSheet, TextInput } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { ThemedText } from '@/components/themed-text';
import { ThemedView } from '@/components/themed-view';
import { Spacing } from '@/constants/theme';
import { useTheme } from '@/hooks/use-theme';
import { useAddStickerToPack, useCreatePack, usePacks } from '@/features/packs/usePacks';
import { WHATSAPP_MAX_EMOJIS_PER_STICKER } from '@/features/packs/types';
import { absoluteUrl } from '@/lib/api';
import { useRender } from '@/lib/queries';

const QUICK_EMOJIS = ['😂', '😍', '🔥', '😢', '😱', '👍', '❤️', '😎', '🤣', '😡', '🎉', '👀'];

export default function PreviewScreen() {
  const { renderId } = useLocalSearchParams<{ renderId: string }>();
  const { data: render } = useRender(renderId);
  const theme = useTheme();
  const { data: packs = [] } = usePacks();
  const createPack = useCreatePack();
  const addSticker = useAddStickerToPack();

  const [emojis, setEmojis] = useState<string[]>([]);
  const [newPackName, setNewPackName] = useState('');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  if (!render || render.status === 'failed') {
    return (
      <ThemedView style={styles.center}>
        <ThemedText type="small">{render?.error?.message ?? 'No se pudo generar el sticker.'}</ThemedText>
        <Pressable onPress={() => router.back()} style={styles.retryButton}>
          <ThemedText type="link">Volver a intentar</ThemedText>
        </Pressable>
      </ThemedView>
    );
  }

  if (render.status !== 'ready') {
    return (
      <ThemedView style={styles.center}>
        <ActivityIndicator />
        <ThemedText type="small" themeColor="textSecondary">
          Generando sticker…
        </ThemedText>
      </ThemedView>
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
    <ThemedView style={styles.container}>
      <SafeAreaView style={styles.safeArea} edges={['bottom']}>
        <ThemedView style={styles.previewBox}>
          {render.sticker_url && (
            <Image source={{ uri: absoluteUrl(render.sticker_url) ?? undefined }} style={styles.sticker} contentFit="contain" />
          )}
        </ThemedView>

        <ThemedText type="small" themeColor="textSecondary" style={styles.centerText}>
          {((render.bytes ?? 0) / 1024).toFixed(0)} KB
          {render.fit?.degraded ? ' · calidad reducida para cumplir el límite de WhatsApp' : ''}
        </ThemedText>

        {saved ? (
          <ThemedView type="backgroundElement" style={styles.savedCard}>
            <ThemedText type="smallBold">¡Guardado en el pack!</ThemedText>
            <Pressable onPress={() => router.dismissTo('/packs')}>
              <ThemedText type="linkPrimary">Ir a Mis packs</ThemedText>
            </Pressable>
          </ThemedView>
        ) : (
          <>
            <ThemedText type="small">Emojis (hasta 3)</ThemedText>
            <ThemedView style={styles.emojiRow}>
              {QUICK_EMOJIS.map((emoji) => (
                <Pressable
                  key={emoji}
                  onPress={() => toggleEmoji(emoji)}
                  style={[
                    styles.emojiButton,
                    { backgroundColor: emojis.includes(emoji) ? theme.backgroundSelected : 'transparent' },
                  ]}>
                  <ThemedText type="title" style={styles.emojiText}>
                    {emoji}
                  </ThemedText>
                </Pressable>
              ))}
            </ThemedView>

            <ThemedText type="small">Guardar en un pack</ThemedText>
            {compatiblePacks.map((pack) => (
              <Pressable
                key={pack.identifier}
                onPress={() => saveToPack(pack.identifier)}
                disabled={saving || pack.stickers.length >= 30}
                style={({ pressed }) => [
                  styles.packRow,
                  { opacity: pressed || saving ? 0.7 : 1 },
                ]}>
                <ThemedText>{pack.name}</ThemedText>
                <ThemedText themeColor="textSecondary">{pack.stickers.length}/30</ThemedText>
              </Pressable>
            ))}

            <ThemedView style={styles.newPackRow}>
              <TextInput
                value={newPackName}
                onChangeText={setNewPackName}
                placeholder="Nombre de un pack nuevo"
                placeholderTextColor={theme.textSecondary}
                style={[styles.input, { color: theme.text, borderColor: theme.backgroundSelected }]}
              />
              <Pressable
                onPress={createAndSave}
                disabled={saving}
                style={[styles.createButton, { backgroundColor: theme.text, opacity: saving ? 0.7 : 1 }]}>
                <ThemedText type="smallBold" themeColor="background">
                  Crear
                </ThemedText>
              </Pressable>
            </ThemedView>
          </>
        )}
      </SafeAreaView>
    </ThemedView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  safeArea: { flex: 1, padding: Spacing.three, gap: Spacing.three },
  center: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: Spacing.two, padding: Spacing.four },
  centerText: { textAlign: 'center' },
  previewBox: {
    aspectRatio: 1,
    borderRadius: Spacing.three,
    overflow: 'hidden',
    // Simple checkerboard-ish neutral backdrop so a transparent sticker
    // (any static/animated webp with alpha) is visible against something.
    backgroundColor: '#80808022',
    alignItems: 'center',
    justifyContent: 'center',
  },
  sticker: { width: '80%', height: '80%' },
  emojiRow: { flexDirection: 'row', flexWrap: 'wrap', gap: Spacing.one },
  emojiButton: { padding: Spacing.one, borderRadius: Spacing.two },
  emojiText: { fontSize: 28, lineHeight: 34 },
  packRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    paddingVertical: Spacing.two,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: '#80808044',
  },
  newPackRow: { flexDirection: 'row', gap: Spacing.two, marginTop: Spacing.one },
  input: {
    flex: 1,
    borderWidth: 1,
    borderRadius: Spacing.two,
    paddingHorizontal: Spacing.three,
    paddingVertical: Spacing.two,
  },
  createButton: {
    paddingHorizontal: Spacing.four,
    borderRadius: Spacing.two,
    alignItems: 'center',
    justifyContent: 'center',
  },
  savedCard: { padding: Spacing.four, borderRadius: Spacing.three, gap: Spacing.two, alignItems: 'center' },
  retryButton: { marginTop: Spacing.two },
});

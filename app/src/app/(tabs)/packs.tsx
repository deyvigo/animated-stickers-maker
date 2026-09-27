import { Image } from 'expo-image';
import { router } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useEffect, useState } from 'react';
import { Alert, FlatList, Pressable, StyleSheet, Text, View } from 'react-native';
import Animated, {
  Easing,
  useAnimatedStyle,
  useSharedValue,
  withRepeat,
  withSequence,
  withTiming,
} from 'react-native-reanimated';
import { SafeAreaView } from 'react-native-safe-area-context';

import { BottomTabInset, Kiosk, Spacing } from '@/constants/theme';
import { packAssetUri } from '@/features/packs/storage';
import { canSubmitToWhatsApp, StickerPack, WHATSAPP_MIN_STICKERS_PER_PACK } from '@/features/packs/types';
import { useDeletePack, usePacks } from '@/features/packs/usePacks';
import WhatsappStickers from '@modules/whatsapp-stickers/src/WhatsappStickersModule';

export default function PacksScreen() {
  const { data: packs = [], isLoading } = usePacks();

  return (
    <View style={styles.container}>
      <StatusBar style="light" />
      <SafeAreaView style={styles.safeArea}>
        <Text style={styles.header}>Mis packs</Text>
        {!isLoading && packs.length === 0 && (
          <View style={styles.emptyCard}>
            <Text style={styles.emptyText}>
              Todavía no guardaste ningún sticker. Creá uno desde la pestaña &quot;Nuevo&quot;.
            </Text>
          </View>
        )}
        <FlatList
          data={packs}
          keyExtractor={(p) => p.identifier}
          contentContainerStyle={styles.list}
          renderItem={({ item }) => <PackCard pack={item} />}
        />
      </SafeAreaView>
    </View>
  );
}

function PackCard({ pack }: { pack: StickerPack }) {
  const deletePack = useDeletePack();
  const [adding, setAdding] = useState(false);
  const ready = canSubmitToWhatsApp(pack);
  const pulse = useSharedValue(1);

  // Same "kiosk sign warming up" moment as the Continuar button on Nuevo.
  useEffect(() => {
    if (adding) {
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
  }, [adding, pulse]);

  const pulseStyle = useAnimatedStyle(() => ({ opacity: pulse.value }));

  const handleAdd = async () => {
    setAdding(true);
    try {
      const { consumer, business } = await WhatsappStickers.isWhatsAppInstalled();
      if (!consumer && !business) {
        Alert.alert('WhatsApp no está instalado', 'Instalá WhatsApp para poder agregar el pack.');
        return;
      }
      const result = await WhatsappStickers.addPackToWhatsApp(pack.identifier);
      if (result === 'added') {
        Alert.alert('¡Listo!', 'Abrí WhatsApp para marcar tus stickers como favoritos.');
      }
    } catch (e) {
      Alert.alert('No se pudo agregar el pack', e instanceof Error ? e.message : String(e));
    } finally {
      setAdding(false);
    }
  };

  const handleDelete = () => {
    Alert.alert('Eliminar pack', `¿Borrar "${pack.name}" y sus ${pack.stickers.length} stickers?`, [
      { text: 'Cancelar', style: 'cancel' },
      { text: 'Eliminar', style: 'destructive', onPress: () => deletePack.mutate(pack.identifier) },
    ]);
  };

  return (
    <View style={styles.card}>
      <Pressable
        onPress={() => router.push({ pathname: '/pack-detail', params: { packId: pack.identifier } })}
        style={({ pressed }) => [styles.cardHeader, { opacity: pressed ? 0.7 : 1 }]}>
        {pack.trayImageFile ? (
          <Image
            source={{ uri: packAssetUri(pack.identifier, pack.trayImageFile) }}
            style={styles.tray}
          />
        ) : (
          <View style={[styles.tray, styles.trayPlaceholder]} />
        )}
        <View style={styles.cardHeaderText}>
          <Text style={styles.packName}>{pack.name}</Text>
          <Text style={styles.packMeta}>
            {pack.animatedStickerPack ? 'Animado' : 'Foto'} · {pack.stickers.length}/30
          </Text>
        </View>
      </Pressable>

      {!ready && (
        <Text style={styles.packMeta}>
          Faltan {WHATSAPP_MIN_STICKERS_PER_PACK - pack.stickers.length} stickers para poder
          agregarlo a WhatsApp (mínimo {WHATSAPP_MIN_STICKERS_PER_PACK}).
        </Text>
      )}

      <View style={styles.actions}>
        <Animated.View style={[styles.primaryButtonWrap, pulseStyle]}>
          <Pressable
            onPress={handleAdd}
            disabled={!ready || adding}
            style={({ pressed }) => [
              styles.primaryButton,
              { opacity: !ready ? 0.4 : pressed ? 0.85 : 1 },
            ]}>
            <Text style={styles.primaryButtonLabel}>
              {adding ? 'Agregando…' : 'Añadir a WhatsApp'}
            </Text>
          </Pressable>
        </Animated.View>
        <Pressable
          onPress={handleDelete}
          style={({ pressed }) => [styles.secondaryButton, { opacity: pressed ? 0.7 : 1 }]}>
          <Text style={styles.secondaryButtonLabel}>Eliminar</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: Kiosk.background },
  safeArea: { flex: 1, paddingHorizontal: Spacing.four, gap: Spacing.three },
  header: {
    paddingTop: Spacing.three,
    fontSize: 28,
    fontWeight: '700',
    letterSpacing: -0.4,
    color: Kiosk.text,
  },
  emptyCard: {
    backgroundColor: Kiosk.surface,
    borderWidth: 1,
    borderColor: Kiosk.border,
    borderRadius: 20,
    padding: Spacing.four,
  },
  emptyText: { fontSize: 15, lineHeight: 21, color: Kiosk.textSecondary },
  list: { gap: Spacing.three, paddingBottom: BottomTabInset + Spacing.four },
  card: {
    backgroundColor: Kiosk.surface,
    borderWidth: 1,
    borderColor: Kiosk.border,
    borderRadius: 18,
    padding: Spacing.three,
    gap: Spacing.two,
  },
  cardHeader: { flexDirection: 'row', gap: Spacing.three, alignItems: 'center' },
  cardHeaderText: { gap: 2, flex: 1 },
  packName: { fontSize: 15, fontWeight: '700', color: Kiosk.text },
  packMeta: { fontSize: 13, color: Kiosk.textSecondary },
  tray: { width: 48, height: 48, borderRadius: Spacing.two },
  trayPlaceholder: { backgroundColor: Kiosk.inset },
  actions: { flexDirection: 'row', gap: Spacing.two, marginTop: Spacing.one },
  primaryButtonWrap: { flex: 1 },
  primaryButton: {
    paddingVertical: Spacing.two,
    borderRadius: 12,
    alignItems: 'center',
    backgroundColor: Kiosk.accent,
  },
  primaryButtonLabel: { fontSize: 14, fontWeight: '700', color: Kiosk.onAccent },
  secondaryButton: {
    paddingVertical: Spacing.two,
    paddingHorizontal: Spacing.three,
    borderRadius: 12,
    alignItems: 'center',
    borderWidth: 1,
    borderColor: Kiosk.border,
  },
  secondaryButtonLabel: { fontSize: 14, fontWeight: '600', color: Kiosk.error },
});

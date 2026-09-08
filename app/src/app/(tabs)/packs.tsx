import { Image } from 'expo-image';
import { useState } from 'react';
import { Alert, FlatList, Pressable, StyleSheet } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { ThemedText } from '@/components/themed-text';
import { ThemedView } from '@/components/themed-view';
import { BottomTabInset, Spacing } from '@/constants/theme';
import { packAssetUri } from '@/features/packs/storage';
import { canSubmitToWhatsApp, StickerPack, WHATSAPP_MIN_STICKERS_PER_PACK } from '@/features/packs/types';
import { useDeletePack, usePacks } from '@/features/packs/usePacks';
import WhatsappStickers from '@modules/whatsapp-stickers/src/WhatsappStickersModule';

export default function PacksScreen() {
  const { data: packs = [], isLoading } = usePacks();

  return (
    <ThemedView style={styles.container}>
      <SafeAreaView style={styles.safeArea}>
        <ThemedText type="subtitle" style={styles.header}>
          Mis packs
        </ThemedText>
        {!isLoading && packs.length === 0 && (
          <ThemedView type="backgroundElement" style={styles.emptyCard}>
            <ThemedText type="small" themeColor="textSecondary">
              Todavía no guardaste ningún sticker. Creá uno desde la pestaña &quot;Nuevo&quot;.
            </ThemedText>
          </ThemedView>
        )}
        <FlatList
          data={packs}
          keyExtractor={(p) => p.identifier}
          contentContainerStyle={styles.list}
          renderItem={({ item }) => <PackCard pack={item} />}
        />
      </SafeAreaView>
    </ThemedView>
  );
}

function PackCard({ pack }: { pack: StickerPack }) {
  const deletePack = useDeletePack();
  const [adding, setAdding] = useState(false);
  const ready = canSubmitToWhatsApp(pack);

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
    <ThemedView type="backgroundElement" style={styles.card}>
      <ThemedView style={styles.cardHeader}>
        {pack.trayImageFile ? (
          <Image
            source={{ uri: packAssetUri(pack.identifier, pack.trayImageFile) }}
            style={styles.tray}
          />
        ) : (
          <ThemedView type="backgroundSelected" style={styles.tray} />
        )}
        <ThemedView style={styles.cardHeaderText}>
          <ThemedText type="smallBold">{pack.name}</ThemedText>
          <ThemedText type="small" themeColor="textSecondary">
            {pack.animatedStickerPack ? 'Animado' : 'Foto'} · {pack.stickers.length}/30
          </ThemedText>
        </ThemedView>
      </ThemedView>

      {!ready && (
        <ThemedText type="small" themeColor="textSecondary">
          Faltan {WHATSAPP_MIN_STICKERS_PER_PACK - pack.stickers.length} stickers para poder
          agregarlo a WhatsApp (mínimo {WHATSAPP_MIN_STICKERS_PER_PACK}).
        </ThemedText>
      )}

      <ThemedView style={styles.actions}>
        <Pressable
          onPress={handleAdd}
          disabled={!ready || adding}
          style={({ pressed }) => [
            styles.primaryButton,
            { opacity: !ready ? 0.4 : pressed || adding ? 0.7 : 1 },
          ]}>
          <ThemedText type="smallBold" themeColor="background">
            {adding ? 'Agregando…' : 'Añadir a WhatsApp'}
          </ThemedText>
        </Pressable>
        <Pressable onPress={handleDelete} style={styles.secondaryButton}>
          <ThemedText type="small">Eliminar</ThemedText>
        </Pressable>
      </ThemedView>
    </ThemedView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  safeArea: { flex: 1, paddingHorizontal: Spacing.four, gap: Spacing.three },
  header: { paddingTop: Spacing.three },
  emptyCard: { padding: Spacing.four, borderRadius: Spacing.three },
  list: { gap: Spacing.three, paddingBottom: BottomTabInset + Spacing.four },
  card: { borderRadius: Spacing.three, padding: Spacing.three, gap: Spacing.two, marginBottom: Spacing.three },
  cardHeader: { flexDirection: 'row', gap: Spacing.three, alignItems: 'center' },
  cardHeaderText: { gap: 2, flex: 1 },
  tray: { width: 48, height: 48, borderRadius: Spacing.two },
  actions: { flexDirection: 'row', gap: Spacing.two, marginTop: Spacing.one },
  primaryButton: {
    flex: 1,
    paddingVertical: Spacing.two,
    borderRadius: Spacing.two,
    alignItems: 'center',
    backgroundColor: '#3c87f7',
  },
  secondaryButton: {
    paddingVertical: Spacing.two,
    paddingHorizontal: Spacing.three,
    borderRadius: Spacing.two,
    alignItems: 'center',
  },
});

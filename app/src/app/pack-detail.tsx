import { Image } from 'expo-image';
import { useLocalSearchParams } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { useState } from 'react';
import { FlatList, Pressable, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Kiosk, Spacing } from '@/constants/theme';
import { packAssetUri } from '@/features/packs/storage';
import { StickerActionSheet } from '@/features/packs/StickerActionSheet';
import { StickerFile, WHATSAPP_MAX_STICKERS_PER_PACK } from '@/features/packs/types';
import { usePacks } from '@/features/packs/usePacks';

const GRID_COLUMNS = 3;

export default function PackDetailScreen() {
  const { packId } = useLocalSearchParams<{ packId: string }>();
  const { data: packs = [] } = usePacks();
  const pack = packs.find((p) => p.identifier === packId);
  const [selectedSticker, setSelectedSticker] = useState<StickerFile | null>(null);

  if (!pack) {
    return (
      <View style={[styles.container, styles.center]}>
        <StatusBar style="light" />
        <Text style={styles.emptyText}>No se encontró el pack.</Text>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <StatusBar style="light" />
      <SafeAreaView style={styles.safeArea} edges={['bottom']}>
        <View style={styles.header}>
          <Text style={styles.title}>{pack.name}</Text>
          <Text style={styles.meta}>
            {pack.stickers.length}/{WHATSAPP_MAX_STICKERS_PER_PACK} stickers ·{' '}
            {pack.animatedStickerPack ? 'Animado' : 'Foto'}
          </Text>
        </View>

        <FlatList
          data={pack.stickers}
          keyExtractor={(s) => s.imageFile}
          numColumns={GRID_COLUMNS}
          columnWrapperStyle={styles.row}
          contentContainerStyle={styles.grid}
          renderItem={({ item }) => (
            <StickerCell packId={pack.identifier} sticker={item} onPress={() => setSelectedSticker(item)} />
          )}
        />
      </SafeAreaView>

      <StickerActionSheet
        packId={pack.identifier}
        sticker={selectedSticker}
        onClose={() => setSelectedSticker(null)}
      />
    </View>
  );
}

function StickerCell({
  packId,
  sticker,
  onPress,
}: {
  packId: string;
  sticker: StickerFile;
  onPress: () => void;
}) {
  return (
    <Pressable onPress={onPress} style={({ pressed }) => [styles.cell, { opacity: pressed ? 0.7 : 1 }]}>
      <Image source={{ uri: packAssetUri(packId, sticker.imageFile) }} style={styles.stickerImage} contentFit="contain" />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: Kiosk.background },
  safeArea: { flex: 1, paddingHorizontal: Spacing.four, gap: Spacing.three },
  center: { alignItems: 'center', justifyContent: 'center', padding: Spacing.four },
  header: { paddingTop: Spacing.three, gap: 2 },
  title: { fontSize: 22, fontWeight: '700', letterSpacing: -0.3, color: Kiosk.text },
  meta: { fontSize: 13, color: Kiosk.textSecondary },
  emptyText: { fontSize: 15, color: Kiosk.textSecondary },
  grid: { gap: Spacing.two, paddingBottom: Spacing.four },
  row: { gap: Spacing.two },
  cell: {
    flex: 1 / GRID_COLUMNS,
    aspectRatio: 1,
    borderRadius: Spacing.two,
    overflow: 'hidden',
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: Kiosk.inset,
    borderWidth: 1,
    borderColor: Kiosk.border,
  },
  stickerImage: { width: '100%', height: '100%' },
});

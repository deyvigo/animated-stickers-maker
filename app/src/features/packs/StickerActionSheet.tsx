import { Image } from 'expo-image';
import { useEffect, useState } from 'react';
import { Alert, BackHandler, Pressable, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import Animated, {
  Easing,
  Extrapolation,
  interpolate,
  runOnJS,
  useAnimatedStyle,
  useSharedValue,
  withTiming,
} from 'react-native-reanimated';

import { Portal } from '@/components/portal';
import { Kiosk, Spacing } from '@/constants/theme';
import { packAssetUri } from '@/features/packs/storage';
import { StickerFile } from '@/features/packs/types';
import { useRemoveSticker } from '@/features/packs/usePacks';

// Fixed, generous off-screen offset — simpler and just as robust as
// measuring the sheet's real height, since the content here is short and
// fixed (thumbnail + one button).
const OFFSET = 420;

/** Bottom sheet for one sticker inside a pack, mounted through a <Portal>
 * so it paints above the pack-detail screen (and the tab bar behind it),
 * with its own slide-up/slide-down animation. `sticker` is the source of
 * truth for what's open; passing `null` starts the close animation, and
 * the sheet keeps rendering the last sticker until that animation finishes. */
export function StickerActionSheet({
  packId,
  sticker,
  onClose,
}: {
  packId: string;
  sticker: StickerFile | null;
  onClose: () => void;
}) {
  const [renderedSticker, setRenderedSticker] = useState<StickerFile | null>(sticker);
  const progress = useSharedValue(0);
  const removeSticker = useRemoveSticker();

  useEffect(() => {
    if (sticker) {
      setRenderedSticker(sticker);
      progress.value = withTiming(1, { duration: 300, easing: Easing.out(Easing.cubic) });
    } else {
      progress.value = withTiming(0, { duration: 220, easing: Easing.in(Easing.cubic) }, (finished) => {
        if (finished) runOnJS(setRenderedSticker)(null);
      });
    }
    // progress/renderedSticker are only ever driven from here — depending on
    // them too would re-trigger this on every animation tick/setState.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sticker]);

  // While the sheet is open, Android's back button/gesture closes it
  // instead of leaving the screen underneath.
  useEffect(() => {
    if (!sticker) return;
    const sub = BackHandler.addEventListener('hardwareBackPress', () => {
      onClose();
      return true;
    });
    return () => sub.remove();
  }, [sticker, onClose]);

  const insets = useSafeAreaInsets();

  const backdropStyle = useAnimatedStyle(() => ({
    opacity: interpolate(progress.value, [0, 1], [0, 0.6], Extrapolation.CLAMP),
  }));
  const sheetStyle = useAnimatedStyle(() => ({
    transform: [{ translateY: interpolate(progress.value, [0, 1], [OFFSET, 0], Extrapolation.CLAMP) }],
  }));

  if (!renderedSticker) return null;

  const handleDelete = () => {
    Alert.alert('Eliminar sticker', '¿Sacar este sticker del pack?', [
      { text: 'Cancelar', style: 'cancel' },
      {
        text: 'Eliminar',
        style: 'destructive',
        onPress: () => {
          removeSticker.mutate(
            { packId, imageFile: renderedSticker.imageFile },
            {
              onSuccess: onClose,
              onError: (e) => Alert.alert('No se pudo eliminar', e instanceof Error ? e.message : String(e)),
            },
          );
        },
      },
    ]);
  };

  return (
    <Portal>
      <Animated.View style={[styles.backdrop, backdropStyle]}>
        <Pressable style={StyleSheet.absoluteFill} onPress={onClose} />
      </Animated.View>
      <Animated.View
        style={[styles.sheet, sheetStyle, { paddingBottom: Math.max(insets.bottom, Spacing.four) }]}>
        <View style={styles.handle} />

        <View style={styles.preview}>
          <Image
            source={{ uri: packAssetUri(packId, renderedSticker.imageFile) }}
            style={styles.stickerImage}
            contentFit="contain"
          />
        </View>
        {renderedSticker.emojis.length > 0 && (
          <Text style={styles.emojis}>{renderedSticker.emojis.join(' ')}</Text>
        )}

        <Pressable
          onPress={handleDelete}
          disabled={removeSticker.isPending}
          style={({ pressed }) => [
            styles.deleteButton,
            { opacity: pressed || removeSticker.isPending ? 0.7 : 1 },
          ]}>
          <Text style={styles.deleteLabel}>
            {removeSticker.isPending ? 'Eliminando…' : 'Eliminar de este pack'}
          </Text>
        </Pressable>
      </Animated.View>
    </Portal>
  );
}

const styles = StyleSheet.create({
  backdrop: { ...StyleSheet.absoluteFill, backgroundColor: '#000000' },
  sheet: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: Kiosk.surface,
    borderTopWidth: 1,
    borderColor: Kiosk.border,
    borderTopLeftRadius: 24,
    borderTopRightRadius: 24,
    paddingTop: Spacing.two,
    paddingHorizontal: Spacing.four,
    gap: Spacing.three,
    alignItems: 'center',
    shadowColor: '#000',
    shadowOffset: { width: 0, height: -4 },
    shadowOpacity: 0.35,
    shadowRadius: 16,
    elevation: 12,
  },
  handle: {
    width: 36,
    height: 4,
    borderRadius: 2,
    backgroundColor: Kiosk.border,
  },
  preview: {
    width: 160,
    height: 160,
    borderRadius: Spacing.three,
    backgroundColor: Kiosk.inset,
    borderWidth: 1,
    borderColor: Kiosk.border,
    alignItems: 'center',
    justifyContent: 'center',
    overflow: 'hidden',
  },
  stickerImage: { width: '80%', height: '80%' },
  emojis: { fontSize: 20, color: Kiosk.text },
  deleteButton: {
    alignSelf: 'stretch',
    paddingVertical: Spacing.three,
    borderRadius: 14,
    alignItems: 'center',
    borderWidth: 1,
    borderColor: Kiosk.border,
  },
  deleteLabel: { fontSize: 15, fontWeight: '700', color: Kiosk.error },
});

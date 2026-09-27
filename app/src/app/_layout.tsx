import { QueryClientProvider } from '@tanstack/react-query';
import { DarkTheme, DefaultTheme, Stack, ThemeProvider } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { useColorScheme } from 'react-native';
import { GestureHandlerRootView } from 'react-native-gesture-handler';

import { AnimatedSplashOverlay } from '@/components/animated-icon';
import { PortalProvider } from '@/components/portal';
import { Kiosk } from '@/constants/theme';
import { queryClient } from '@/lib/queryClient';

SplashScreen.preventAutoHideAsync();

// The "kiosco nocturno" palette (see Kiosk in constants/theme.ts) now covers
// every screen reachable from Nuevo → editor → preview and from Mis packs →
// pack-detail, so their native headers match instead of following the
// system light/dark theme like the Stack's own ThemeProvider below.
const kioskHeaderOptions = {
  headerStyle: { backgroundColor: Kiosk.background },
  headerTintColor: Kiosk.text,
};

export default function RootLayout() {
  const colorScheme = useColorScheme();
  return (
    // Required by react-native-gesture-handler: any GestureDetector (used in
    // the editor's TrimTimeline/ZoomableVideoCrop) needs an ancestor
    // GestureHandlerRootView, or gestures silently fail to register.
    <GestureHandlerRootView style={{ flex: 1 }}>
      <PortalProvider>
        <QueryClientProvider client={queryClient}>
          <ThemeProvider value={colorScheme === 'dark' ? DarkTheme : DefaultTheme}>
            <AnimatedSplashOverlay />
            <Stack screenOptions={{ headerShown: false }}>
              <Stack.Screen name="(tabs)" />
              <Stack.Screen
                name="editor"
                options={{
                  headerShown: true,
                  title: 'Recortar',
                  presentation: 'modal',
                  ...kioskHeaderOptions,
                }}
              />
              <Stack.Screen
                name="preview"
                options={{
                  headerShown: true,
                  title: 'Sticker',
                  presentation: 'modal',
                  ...kioskHeaderOptions,
                }}
              />
              <Stack.Screen
                name="pack-detail"
                options={{ headerShown: true, title: 'Pack', ...kioskHeaderOptions }}
              />
            </Stack>
          </ThemeProvider>
        </QueryClientProvider>
      </PortalProvider>
    </GestureHandlerRootView>
  );
}

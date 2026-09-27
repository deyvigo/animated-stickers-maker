/**
 * Below are the colors that are used in the app. The colors are defined in the light and dark mode.
 * There are many other ways to style your app. For example, [Nativewind](https://www.nativewind.dev/), [Tamagui](https://tamagui.dev/), [unistyles](https://reactnativeunistyles.vercel.app), etc.
 */

import '@/global.css';

import { Platform } from 'react-native';

export const Colors = {
  light: {
    text: '#000000',
    background: '#ffffff',
    backgroundElement: '#F0F0F3',
    backgroundSelected: '#E0E1E6',
    textSecondary: '#60646C',
  },
  dark: {
    text: '#ffffff',
    background: '#000000',
    backgroundElement: '#212225',
    backgroundSelected: '#2E3135',
    textSecondary: '#B0B4BA',
  },
} as const;

export type ThemeColor = keyof typeof Colors.light & keyof typeof Colors.dark;

/**
 * "Kiosco nocturno" — the corner phone-repair/copy kiosk at night. Pinned
 * dark palette shared by the home, packs, and pack-detail screens; not tied
 * to the system light/dark scheme like `Colors` above. See
 * .impeccable/surfaces/app-src-app-tabs-index-tsx.md for the direction
 * contract this implements.
 */
export const Kiosk = {
  // Pinned to Colors.dark.background (not a Kiosk-only hex) so these screens'
  // background always matches the native bottom tab bar, which reads its
  // own color from Colors[scheme] in app-tabs.tsx.
  background: Colors.dark.background,
  surface: '#141517',
  // A touch darker than `surface`: the "cut into the case" slot color for
  // inputs, image placeholders, and grid cells sitting inside a card.
  inset: '#0F1011',
  border: 'rgba(194, 231, 218, 0.24)',
  borderFocused: 'rgba(194, 231, 218, 0.6)',
  accent: '#C2E7DA',
  onAccent: '#0B0C0D',
  text: '#F2F1ED',
  textSecondary: '#7C948C',
  error: '#FF6B5E',
} as const;

export const Fonts = Platform.select({
  ios: {
    /** iOS `UIFontDescriptorSystemDesignDefault` */
    sans: 'system-ui',
    /** iOS `UIFontDescriptorSystemDesignSerif` */
    serif: 'ui-serif',
    /** iOS `UIFontDescriptorSystemDesignRounded` */
    rounded: 'ui-rounded',
    /** iOS `UIFontDescriptorSystemDesignMonospaced` */
    mono: 'ui-monospace',
  },
  default: {
    sans: 'normal',
    serif: 'serif',
    rounded: 'normal',
    mono: 'monospace',
  },
  web: {
    sans: 'var(--font-display)',
    serif: 'var(--font-serif)',
    rounded: 'var(--font-rounded)',
    mono: 'var(--font-mono)',
  },
});

export const Spacing = {
  half: 2,
  one: 4,
  two: 8,
  three: 16,
  four: 24,
  five: 32,
  six: 64,
} as const;

export const BottomTabInset = Platform.select({ ios: 50, android: 80 }) ?? 0;
export const MaxContentWidth = 800;

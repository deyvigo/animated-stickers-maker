import type { ExpoConfig } from "expo/config";

// Dynamic config (instead of app.json) so we can read EXPO_PUBLIC_API_URL
// at prebuild time: Android blocks cleartext (http://) traffic by default
// starting with API 28, which would silently break every request to a
// backend reached over plain http:// on your LAN during development.
const apiUrl = process.env.EXPO_PUBLIC_API_URL ?? "";
const usesCleartextTraffic = apiUrl.startsWith("http://");

const config: ExpoConfig = {
  name: "Stickers Maker",
  slug: "animated-stickers-maker",
  version: "1.0.0",
  orientation: "portrait",
  icon: "./assets/images/icon.png",
  scheme: "stickersmaker",
  userInterfaceStyle: "automatic",
  ios: {
    icon: "./assets/expo.icon",
    bundleIdentifier: "com.deyvigo.stickersmaker",
  },
  android: {
    package: "com.deyvigo.stickersmaker",
    adaptiveIcon: {
      backgroundColor: "#E6F4FE",
      foregroundImage: "./assets/images/android-icon-foreground.png",
      backgroundImage: "./assets/images/android-icon-background.png",
      monochromeImage: "./assets/images/android-icon-monochrome.png",
    },
    predictiveBackGestureEnabled: false,
  },
  web: {
    output: "static",
    favicon: "./assets/images/favicon.png",
  },
  plugins: [
    "expo-router",
    [
      "expo-splash-screen",
      {
        backgroundColor: "#208AEF",
        image: "./assets/images/splash-icon.png",
        imageWidth: 76,
      },
    ],
    "expo-video",
    "expo-sqlite",
    "expo-sharing",
    [
      "expo-build-properties",
      {
        // usesCleartextTraffic isn't a top-level `android.*` Expo config
        // field (an earlier version of this file put it there — Expo
        // silently ignores unknown keys instead of erroring, so that never
        // actually applied). This plugin is the real, documented way to
        // set it on the AndroidManifest.
        android: { usesCleartextTraffic },
      },
    ],
  ],
  experiments: {
    typedRoutes: true,
    reactCompiler: true,
  },
  extra: {
    eas: {
      projectId: "b1a8f0da-1121-46f3-9006-14b9794e9fba",
    },
  },
};

export default config;

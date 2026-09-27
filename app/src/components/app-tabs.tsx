import { NativeTabs } from 'expo-router/unstable-native-tabs';

import { Kiosk } from '@/constants/theme';

export default function AppTabs() {
  return (
    <NativeTabs
      backgroundColor={Kiosk.background}
      indicatorColor={Kiosk.border}
      iconColor={{ default: Kiosk.textSecondary, selected: Kiosk.accent }}
      labelStyle={{
        default: { color: Kiosk.textSecondary },
        selected: { color: Kiosk.accent },
      }}>
      <NativeTabs.Trigger name="index">
        <NativeTabs.Trigger.Label>Nuevo</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon
          src={{
            default: require('@/assets/images/tabIcons/new.png'),
            selected: require('@/assets/images/tabIcons/new-filled.png'),
          }}
          renderingMode="template"
        />
      </NativeTabs.Trigger>

      <NativeTabs.Trigger name="packs">
        <NativeTabs.Trigger.Label>Mis packs</NativeTabs.Trigger.Label>
        <NativeTabs.Trigger.Icon
          src={{
            default: require('@/assets/images/tabIcons/packs.png'),
            selected: require('@/assets/images/tabIcons/packs-filled.png'),
          }}
          renderingMode="template"
        />
      </NativeTabs.Trigger>
    </NativeTabs>
  );
}

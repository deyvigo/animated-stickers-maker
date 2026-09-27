import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { StyleSheet, View } from 'react-native';

interface PortalContextValue {
  mount: (key: string, node: ReactNode) => void;
  unmount: (key: string) => void;
}

const PortalContext = createContext<PortalContextValue | null>(null);

/** Mounted once at the app root (see _layout.tsx). Renders its own children
 * first, then every currently-mounted <Portal> node on top of them, each in
 * an absolutely-positioned layer — so a Portal's content paints above the
 * whole navigator (tab bar included) regardless of where in the tree the
 * <Portal> itself lives. */
export function PortalProvider({ children }: { children: ReactNode }) {
  const [entries, setEntries] = useState<Record<string, ReactNode>>({});

  const mount = useCallback((key: string, node: ReactNode) => {
    setEntries((prev) => ({ ...prev, [key]: node }));
  }, []);

  const unmount = useCallback((key: string) => {
    setEntries((prev) => {
      if (!(key in prev)) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }, []);

  const value = useMemo(() => ({ mount, unmount }), [mount, unmount]);

  return (
    <PortalContext.Provider value={value}>
      <View style={styles.root}>
        {children}
        {Object.entries(entries).map(([key, node]) => (
          <View key={key} style={StyleSheet.absoluteFill} pointerEvents="box-none">
            {node}
          </View>
        ))}
      </View>
    </PortalContext.Provider>
  );
}

let nextPortalId = 0;

/** Renders `children` at the app root instead of in place — for anything
 * that must paint above the navigator, like a bottom sheet. Requires a
 * <PortalProvider> ancestor (mounted once in _layout.tsx). */
export function Portal({ children }: { children: ReactNode }) {
  const ctx = useContext(PortalContext);
  const idRef = useRef<string | null>(null);
  if (idRef.current === null) idRef.current = `portal-${nextPortalId++}`;
  const id = idRef.current;

  useEffect(() => {
    if (!ctx) return;
    ctx.mount(id, children);
    return () => ctx.unmount(id);
  });

  return null;
}

const styles = StyleSheet.create({
  root: { flex: 1 },
});

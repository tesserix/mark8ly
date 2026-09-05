import '../global.css';

import { useEffect, useRef } from 'react';
import { View } from 'react-native';
import { Stack, useRouter, useSegments } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { BottomSheetModalProvider } from '@gorhom/bottom-sheet';
import {
  QueryClient,
  QueryClientProvider,
  useQueryClient,
} from '@tanstack/react-query';
import * as SplashScreen from 'expo-splash-screen';
import { useFonts } from 'expo-font';
import { AuthProvider, useAuth } from '@repo/mobile-shared/auth/provider';
import { useTenantStore } from '@repo/mobile-shared/stores/tenant-store';
import { ApiError } from '@repo/mobile-shared/api/client';
import { ErrorBoundary } from '../components/ErrorBoundary';
import { fontMap } from '../lib/fonts';

SplashScreen.preventAutoHideAsync();

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: (failureCount, error) =>
        !(error instanceof ApiError && error.code === 'contract_mismatch') &&
        failureCount < 2,
    },
  },
});

function AuthGate() {
  const { user, loading } = useAuth();
  const segments = useSegments();
  const router = useRouter();
  const qc = useQueryClient();
  const hydrate = useTenantStore((s) => s.hydrate);
  const clearTenant = useTenantStore((s) => s.clear);
  const previousUid = useRef<string | null>(null);

  useEffect(() => {
    hydrate();
  }, [hydrate]);

  useEffect(() => {
    if (loading) return;
    const inAuthGroup = segments[0] === 'login';

    // Identity changed → wipe cached queries + tenant so user A's data can't
    // leak into user B's session.
    const currentUid = user?.uid ?? null;
    if (previousUid.current !== null && previousUid.current !== currentUid) {
      qc.clear();
      clearTenant();
    }
    previousUid.current = currentUid;

    if (!user && !inAuthGroup) {
      router.replace('/login');
    } else if (user && inAuthGroup) {
      router.replace('/');
    }
    SplashScreen.hideAsync();
  }, [user, loading, segments, router, clearTenant, qc]);

  // Root stack (not Slot) so screens outside the tab group — like the
  // notifications inbox — push as cards ABOVE the tabs. Launching the inbox
  // from the dashboard bell no longer corrupts the More tab's saved stack
  // state (which used to leave the More dock button reopening notifications
  // and back popping to the dashboard).
  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="(tabs)" />
      <Stack.Screen name="login" />
      <Stack.Screen name="otp" />
      <Stack.Screen name="notifications" />
    </Stack>
  );
}

export default function RootLayout() {
  const [fontsLoaded] = useFonts(fontMap);
  if (!fontsLoaded) return <View className="flex-1 bg-paper" />;

  return (
    <ErrorBoundary>
      <AuthProvider>
        {/* 🔴 DARK status-bar content, set at RUNTIME and on PURPOSE.
            Nothing set a status-bar style before this, so Android defaulted to
            LIGHT content and painted a white clock/battery/wifi onto the Paper
            background (#F7F6F2) — measured luminance 255 on 245, about 1.04:1,
            i.e. invisible on EVERY screen. iOS already resolved to dark content
            (measured 0), so this makes the two platforms agree rather than
            changing iOS.
            Runtime `<StatusBar>` rather than app.config.js `androidStatusBar`:
            the config route only lands at prebuild, and android/ + ios/ are
            gitignored and regenerated, so a JS-owned value cannot drift out of
            the repo. This app is light-mode only (Paper · Ink · Moss), so dark
            content is correct unconditionally — there is no dark theme to
            branch on. */}
        <StatusBar style="dark" />
        <GestureHandlerRootView style={{ flex: 1 }}>
          <SafeAreaProvider>
            <QueryClientProvider client={queryClient}>
              <BottomSheetModalProvider>
                <AuthGate />
              </BottomSheetModalProvider>
            </QueryClientProvider>
          </SafeAreaProvider>
        </GestureHandlerRootView>
      </AuthProvider>
    </ErrorBoundary>
  );
}

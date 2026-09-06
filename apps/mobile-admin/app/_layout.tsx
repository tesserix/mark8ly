import '../global.css';

import { useEffect, useRef, useState } from 'react';
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
import { zitadelSession } from '@repo/mobile-shared/auth/zitadel-session';
import { isZitadelProvider } from '@/lib/auth-provider';
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
  // Re-read the token whenever the route changes, so completing the OTP
  // screen is noticed without waiting for a remount.
  const segmentsKey = segments.join('/');
  // Under Zitadel `user` is ALWAYS null — that field belongs to the Firebase
  // SDK and this provider never populates it. Signed-in-ness comes from the
  // token this app persisted at sign-in instead. Without this the gate below
  // bounces every Zitadel session straight back to /login: the navigation
  // succeeds, the guard undoes it, and the user sees a blank login form with
  // no error anywhere — the #493 failure, reproduced exactly.
  const [zitadelSignedIn, setZitadelSignedIn] = useState<boolean | null>(
    isZitadelProvider() ? null : false,
  );
  useEffect(() => {
    if (!isZitadelProvider()) return;
    let cancelled = false;
    zitadelSession
      .accessTokenIfFresh()
      .then((t) => {
        if (!cancelled) setZitadelSignedIn(Boolean(t));
      })
      .catch(() => {
        if (!cancelled) setZitadelSignedIn(false);
      });
    return () => {
      cancelled = true;
    };
  }, [segmentsKey]);
  const qc = useQueryClient();
  const hydrate = useTenantStore((s) => s.hydrate);
  const clearTenant = useTenantStore((s) => s.clear);
  const previousUid = useRef<string | null>(null);

  useEffect(() => {
    hydrate();
  }, [hydrate]);

  useEffect(() => {
    if (loading) return;
    // null means the token read has not resolved yet. Redirecting on an
    // unknown answer would race the check and bounce a signed-in user.
    if (zitadelSignedIn === null) return;
    // /otp and /totp are part of the sign-in flow: the caller is
    // legitimately not authenticated there yet, so neither may be treated
    // as a protected route — a step-up screen that bounces to /login is
    // the same lockout, one layer up.
    const inAuthGroup =
      segments[0] === 'login' || segments[0] === 'otp' || segments[0] === 'totp';
    const signedIn = isZitadelProvider() ? zitadelSignedIn : Boolean(user);

    // Identity changed → wipe cached queries + tenant so user A's data can't
    // leak into user B's session.
    const currentUid = user?.uid ?? null;
    if (previousUid.current !== null && previousUid.current !== currentUid) {
      qc.clear();
      clearTenant();
    }
    previousUid.current = currentUid;

    if (!signedIn && !inAuthGroup) {
      router.replace('/login');
    } else if (signedIn && inAuthGroup) {
      router.replace('/');
    }
    SplashScreen.hideAsync();
  }, [user, loading, segments, router, clearTenant, qc, zitadelSignedIn]);

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
      <Stack.Screen name="totp" />
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

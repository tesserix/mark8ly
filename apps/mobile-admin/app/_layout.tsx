import '../global.css';

import { useEffect, useRef } from 'react';
import { View } from 'react-native';
import { Slot, useRouter, useSegments } from 'expo-router';
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
import { ErrorBoundary } from '../components/ErrorBoundary';
import { fontMap } from '../lib/fonts';

SplashScreen.preventAutoHideAsync();

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 2 } },
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

  return <Slot />;
}

export default function RootLayout() {
  const [fontsLoaded] = useFonts(fontMap);
  if (!fontsLoaded) return <View className="flex-1 bg-paper" />;

  return (
    <ErrorBoundary>
      <AuthProvider>
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

import { useEffect, useRef, type ReactNode } from "react";
import {
  ActivityIndicator,
  AppState,
  type AppStateStatus,
  Linking,
  StyleSheet,
  TouchableOpacity,
  View,
} from "react-native";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useEnvironment } from "@repo/mobile-shared/config/env";
import { useTenantResolver } from "@/lib/hooks/use-tenant-resolver";
import { StorePicker } from "@/components/StorePicker";
import { EmptyState, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

/**
 * Blocks the tabs surface until the signed-in user's tenant has been
 * resolved. Three states:
 *
 *   - resolving:        spinner while we fetch + auto-select a store.
 *   - needsOnboarding:  user has no stores → CTA to mark8ly.com to register.
 *   - error:            fetch failed → retry button.
 *   - ready:            render children.
 */
export function TenantGate({ children }: { children: ReactNode }) {
  const env = useEnvironment();
  const queryClient = useQueryClient();
  const {
    resolving,
    needsOnboarding,
    needsPicker,
    availableStores,
    error,
    refetch,
  } = useTenantResolver();
  const lastState = useRef<AppStateStatus>(AppState.currentState);

  useEffect(() => {
    // Re-validate the session whenever the app comes back to foreground.
    // The membership list, role, and dashboard data may have changed
    // while the app was backgrounded; invalidating ["stores"] makes the
    // resolver re-run, and ["dashboard"] / ["orders"] / etc. refetch
    // through useApiClient — so a revoked role triggers the
    // onTenantInvalid hook and lands the user on the right tenant.
    const sub = AppState.addEventListener("change", (next) => {
      if (lastState.current.match(/inactive|background/) && next === "active") {
        queryClient.invalidateQueries();
      }
      lastState.current = next;
    });
    return () => sub.remove();
  }, [queryClient]);

  if (error) {
    // A 403 here is a permissions verdict, not a network problem — "check your
    // connection" would send the user chasing the wrong fix, and retrying a
    // denial just fails again.
    const denied = error instanceof ApiError && error.status === 403;
    return (
      <Screen>
        <View style={styles.center}>
          <EmptyState
            title={denied ? "No access" : "Couldn't load your store"}
            message={
              denied
                ? "That account doesn't have access to a Mark8ly admin account."
                : "Check your connection and try again."
            }
          />
          {denied ? null : (
            <TouchableOpacity
              onPress={() => refetch()}
              style={styles.primaryBtn}
              activeOpacity={0.85}
              accessibilityRole="button"
              accessibilityLabel="Retry"
            >
              <Text preset="bodyEmphasis" color="inverse">
                Retry
              </Text>
            </TouchableOpacity>
          )}
        </View>
      </Screen>
    );
  }

  if (needsPicker) {
    return <StorePicker stores={availableStores} />;
  }

  if (needsOnboarding) {
    return (
      <Screen>
        <View style={styles.center}>
          <EmptyState
            title="No store yet"
            message="Create your first store on mark8ly.com to start selling."
          />
          <TouchableOpacity
            onPress={() => Linking.openURL(env.signupUrl)}
            style={styles.primaryBtn}
            activeOpacity={0.85}
            accessibilityRole="link"
            accessibilityLabel="Open mark8ly.com to create a new store"
          >
            <Text preset="bodyEmphasis" color="inverse">
              Create a new store
            </Text>
          </TouchableOpacity>
        </View>
      </Screen>
    );
  }

  if (resolving) {
    return (
      <Screen>
        <View style={styles.center}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  return <>{children}</>;
}

const styles = StyleSheet.create({
  center: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: theme.spacing.xl,
    gap: theme.spacing.lg,
  },
  primaryBtn: {
    backgroundColor: theme.colors.text,
    height: 48,
    paddingHorizontal: theme.spacing.xl,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
    minWidth: 200,
  },
});

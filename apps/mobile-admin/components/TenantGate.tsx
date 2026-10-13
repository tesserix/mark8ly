import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  ActivityIndicator,
  AppState,
  type AppStateStatus,
  Platform,
  Pressable,
  StyleSheet,
  View,
} from "react-native";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useTenantResolver } from "@/lib/hooks/use-tenant-resolver";
import { StorePicker } from "@/components/StorePicker";
import { EmptyState, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

/**
 * Blocks the tabs surface until the signed-in user's tenant has been
 * resolved. Three states:
 *
 *   - resolving:        spinner while we fetch + auto-select a store.
 *   - needsOnboarding:  user has no stores → ask the owner for an invite.
 *   - error:            fetch failed → retry button.
 *   - ready:            render children.
 */
export function TenantGate({ children }: { children: ReactNode }) {
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
  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [retryPressed, setRetryPressed] = useState(false);

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
          {/* CENTRED, deliberately. Every list screen's empty state is now
              left-aligned, but this is not a list screen — it is a gate that
              renders BEFORE any tenant, header, eyebrow or row exists, so
              there is no established left edge for it to share. Its sibling
              is a centred pill CTA that `styles.center` positions; aligning
              only the copy left would put a left-aligned message above a
              centred button, i.e. two axes where there is currently one.
              Making this left would mean redesigning the gate, which is a
              different change from "make the list screens agree". */}
          <EmptyState
            title={denied ? "No access" : "Couldn't load your store"}
            message={
              denied
                ? "That account doesn't have access to a Mark8ly admin account."
                : "Check your connection and try again."
            }
          />
          {denied ? null : (
            <Pressable
              onPress={() => refetch()}
              onPressIn={() => setRetryPressed(true)}
              onPressOut={() => setRetryPressed(false)}
              accessibilityRole="button"
              accessibilityLabel="Retry"
              android_ripple={theme.press.rippleOnDark}
              style={[
                styles.primaryBtn,
                retryPressed && Platform.OS === "ios" ? { opacity: theme.press.opacitySolidFill } : null,
              ]}
            >
              <Text preset="bodyEmphasis" color="inverse">
                Retry
              </Text>
            </Pressable>
          )}
        </View>
      </Screen>
    );
  }

  if (needsPicker) {
    return <StorePicker stores={availableStores} />;
  }

  if (needsOnboarding) {
    // No link out to the web here, deliberately. App Review reads any button
    // to mark8ly.com from this screen as a call to action to buy a plan
    // outside the app, and that is what Guideline 3.1.3(f) forbids. Staff
    // reach a store by being invited to it, so the copy says that instead.
    return (
      <Screen>
        <View style={styles.center}>
          <EmptyState
            title="No store yet"
            message="This account isn't part of a store. Ask the store owner to invite you, then sign in again."
          />
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

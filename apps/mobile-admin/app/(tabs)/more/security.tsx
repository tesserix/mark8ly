import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Platform, Pressable, StyleSheet, View } from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { authErrorMessage } from "@repo/mobile-shared/auth/errors";
import {
  configureGoogleSignin,
  signInWithAppleNative,
  signInWithGoogleNative,
} from "@/lib/social-auth";
import { BackHeader, GroupedList, GroupedRow, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

const DEMO_AUTH = process.env.EXPO_PUBLIC_AUTH_BACKEND === "demo";

type ProviderId = "password" | "google.com" | "apple.com";
type LinkableProviderId = "google.com" | "apple.com";

function isLinkableProviderId(id: ProviderId): id is LinkableProviderId {
  return id === "google.com" || id === "apple.com";
}

const METHODS: { id: ProviderId; label: string; linkable: boolean }[] = [
  { id: "password", label: "Password", linkable: false },
  { id: "google.com", label: "Google", linkable: true },
  { id: "apple.com", label: "Apple", linkable: true },
];

// Extracted so press state can live in `useState`: this button renders once
// per row inside `METHODS.map()`, and hooks can't be called from inside a
// `.map()` callback — each row needs its own component instance instead.
function MethodActionButton({
  tone,
  label,
  accessibilityLabel,
  disabled,
  onPress,
}: {
  tone: "danger" | "neutral";
  label: string;
  accessibilityLabel: string;
  disabled: boolean;
  onPress: () => void;
}) {
  const [pressed, setPressed] = useState(false);
  return (
    <Pressable
      disabled={disabled}
      onPress={onPress}
      onPressIn={() => setPressed(true)}
      onPressOut={() => setPressed(false)}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      hitSlop={8}
      android_ripple={{
        ...(tone === "danger" ? theme.press.rippleDanger : theme.press.rippleInk),
        borderless: true,
      }}
      style={[
        pressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
      ]}
    >
      <Text preset="bodyEmphasis" color={tone === "danger" ? "danger" : "text"}>
        {label}
      </Text>
    </Pressable>
  );
}

export default function SecurityScreen(): React.JSX.Element {
  const {
    linkedProviderIds,
    linkGoogleToCurrentUser,
    linkAppleToCurrentUser,
    unlinkProvider,
  } = useAuth();
  const [linked, setLinked] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);
  const busyRef = useRef(false);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const refresh = useCallback(async () => {
    const ids = await linkedProviderIds();
    if (mountedRef.current) setLinked(ids);
  }, [linkedProviderIds]);

  useEffect(() => {
    void (async () => {
      try {
        await refresh();
        if (mountedRef.current) setError(null);
      } catch {
        if (mountedRef.current) setError("Couldn't load your sign-in methods.");
      }
    })();
  }, [refresh]);

  const run = useCallback(
    async (fn: () => Promise<void>) => {
      if (busyRef.current) return;
      busyRef.current = true;
      setError(null);
      setBusy(true);
      try {
        await fn();
        try {
          await refresh();
        } catch (e: unknown) {
          const msg = authErrorMessage(e);
          if (msg) setError(msg);
        }
      } catch (e: unknown) {
        const msg = authErrorMessage(e);
        if (msg) setError(msg);
      } finally {
        busyRef.current = false;
        setBusy(false);
      }
    },
    [refresh],
  );

  const handleLink = useCallback(
    (id: LinkableProviderId) => {
      void run(async () => {
        if (id === "google.com") {
          if (DEMO_AUTH) return linkGoogleToCurrentUser("demo-google-token");
          configureGoogleSignin();
          const idToken = await signInWithGoogleNative();
          return linkGoogleToCurrentUser(idToken);
        }
        if (DEMO_AUTH) return linkAppleToCurrentUser("demo-apple-token", "");
        const { idToken, rawNonce } = await signInWithAppleNative();
        return linkAppleToCurrentUser(idToken, rawNonce);
      });
    },
    [run, linkGoogleToCurrentUser, linkAppleToCurrentUser],
  );

  const handleRemove = useCallback(
    (id: ProviderId, label: string) => {
      Alert.alert("Remove sign-in method", `Remove ${label} from your account?`, [
        { text: "Cancel", style: "cancel" },
        {
          text: "Remove",
          style: "destructive",
          onPress: () => void run(() => unlinkProvider(id)),
        },
      ]);
    },
    [run, unlinkProvider],
  );

  return (
    <Screen>
      <BackHeader eyebrow="SECURITY" title="Sign-in methods" />
      <View style={styles.body}>
        <GroupedList
          sections={[
            {
              key: "methods",
              label: "Ways to sign in",
              rows: METHODS.map((m) => {
                const isLinked = linked?.includes(m.id) ?? false;
                const linkableId = isLinkableProviderId(m.id) ? m.id : null;
                const status = linked === null ? "Checking…" : isLinked ? "Connected" : "Not connected";
                return (
                  <GroupedRow
                    key={m.id}
                    label={m.label}
                    hint={status}
                    // No `onPress` — the row itself does nothing; Remove/Link
                    // is a separate press target carried in `trailing`. This
                    // was already a non-interactive View pre-migration, just
                    // one at a 56pt/12pt density instead of every other row
                    // in the app's 64pt/20pt — the real inconsistency this
                    // migration closes.
                    accessibilityLabel={`${m.label}, ${status}`}
                    trailing={
                      linked === null ? undefined : isLinked ? (
                        <MethodActionButton
                          tone="danger"
                          label="Remove"
                          accessibilityLabel={`Remove ${m.label}`}
                          disabled={busy}
                          onPress={() => handleRemove(m.id, m.label)}
                        />
                      ) : m.linkable && linkableId ? (
                        <MethodActionButton
                          tone="neutral"
                          label="Link"
                          accessibilityLabel={`Link ${m.label}`}
                          disabled={busy}
                          onPress={() => handleLink(linkableId)}
                        />
                      ) : undefined
                    }
                  />
                );
              }),
            },
          ]}
        />

        {error ? (
          <View style={styles.errorWrap}>
            <Text
              preset="caption"
              color="danger"
              accessibilityRole="alert"
              accessibilityLiveRegion="polite"
            >
              {error}
            </Text>
          </View>
        ) : null}
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH used
  // by every other row in the app. Not theme.spacing.lg.
  body: { paddingHorizontal: theme.spacing.xl, paddingTop: theme.spacing.xs },
  errorWrap: { paddingTop: theme.spacing.md },
});

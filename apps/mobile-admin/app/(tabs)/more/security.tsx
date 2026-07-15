import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, StyleSheet, TouchableOpacity, View } from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { LastSignInMethodError } from "@repo/mobile-shared/auth/link";
import {
  configureGoogleSignin,
  signInWithAppleNative,
  signInWithGoogleNative,
} from "@/lib/social-auth";
import { BackHeader, Card, Eyebrow, Hairline, Screen, Text } from "@/components/ui";
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

function errorMessage(e: unknown): string {
  if (e instanceof LastSignInMethodError) {
    return "You can't remove your only sign-in method.";
  }
  const code = typeof e === "object" && e !== null ? (e as { code?: unknown }).code : undefined;
  if (code === "auth/credential-already-in-use") {
    return "That account is already linked to a different Mark8ly account.";
  }
  if (code === "auth/provider-already-linked") return "That's already linked to your account.";
  if (code === "auth/requires-recent-login") {
    return "For security, sign out and sign in again, then retry.";
  }
  return e instanceof Error && e.message ? e.message : "Something went wrong. Try again.";
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
          setError(errorMessage(e));
        }
      } catch (e: unknown) {
        setError(errorMessage(e));
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
      <Eyebrow label="Ways to sign in" />
      <Card padding={0} style={styles.card}>
        {METHODS.map((m, i) => {
          const isLinked = linked?.includes(m.id) ?? false;
          const linkableId = isLinkableProviderId(m.id) ? m.id : null;
          return (
            <View key={m.id}>
              {i > 0 ? <Hairline /> : null}
              <View style={styles.row}>
                <View style={styles.rowInfo}>
                  <Text preset="bodyEmphasis" color="text">
                    {m.label}
                  </Text>
                  <Text preset="caption" color="textTertiary">
                    {linked === null ? "Checking…" : isLinked ? "Connected" : "Not connected"}
                  </Text>
                </View>
                {linked === null ? null : isLinked ? (
                  <TouchableOpacity
                    disabled={busy}
                    onPress={() => handleRemove(m.id, m.label)}
                    accessibilityRole="button"
                    accessibilityLabel={`Remove ${m.label}`}
                  >
                    <Text preset="bodyEmphasis" color="danger">
                      Remove
                    </Text>
                  </TouchableOpacity>
                ) : m.linkable && linkableId ? (
                  <TouchableOpacity
                    disabled={busy}
                    onPress={() => handleLink(linkableId)}
                    accessibilityRole="button"
                    accessibilityLabel={`Link ${m.label}`}
                  >
                    <Text preset="bodyEmphasis" color="accent">
                      Link
                    </Text>
                  </TouchableOpacity>
                ) : null}
              </View>
            </View>
          );
        })}
      </Card>

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
    </Screen>
  );
}

const styles = StyleSheet.create({
  card: { marginHorizontal: theme.spacing.lg },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.md,
    minHeight: 56,
  },
  rowInfo: { flex: 1, gap: 2 },
  errorWrap: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.md },
});

import { useState } from "react";
import { Pressable, View } from "react-native";
import { router, useLocalSearchParams } from "expo-router";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { createZitadelSignIn } from "@repo/mobile-shared/auth/zitadel-signin";
import { ZitadelAuthError } from "@repo/mobile-shared/auth/zitadel-client";
import { useEnvironment } from "@repo/mobile-shared/config/env";
import { AuthScreen } from "../components/ui/AuthScreen";
import { CodeInput } from "../components/ui/CodeInput";
import { Text } from "../components/ui/Text";

/**
 * Authenticator-app code screen (#686 item 2).
 *
 * Its absence was a total lockout, not a missing nicety: a merchant with
 * TOTP enrolled got `totp_required` from the login call, the client had no
 * way to resume it, and the login screen told them "this app version needs
 * an update to finish signing in" — advice no update could ever satisfy.
 *
 * It shares AuthScreen and CodeInput with /otp because both collect six
 * digits and must not drift apart. What differs is deliberate and lives in
 * two places only: the copy (nothing here mentions email — the code is not
 * sent anywhere) and CodeInput's `codeSource`, which switches off the
 * delivered-code AutoFill hints that cannot apply to an authenticator.
 */
export default function TotpScreen() {
  const params = useLocalSearchParams<{ pendingToken?: string }>();
  const env = useEnvironment();
  const setTenantId = useTenantStore((s) => s.setTenantId);

  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const pendingToken = params.pendingToken ?? "";

  /**
   * `submitted` is the value CodeInput reports the moment its last cell
   * fills. Reading `code` there would read the previous render's state and
   * verify a five-digit code.
   */
  async function handleVerify(submitted?: string) {
    if (submitting) return;
    const entered = (submitted ?? code).trim();
    if (entered.length === 0) {
      setError("Enter the code from your authenticator app.");
      return;
    }
    // Without this there is nothing to resume, and submitting would fail
    // with copy that blames a code the merchant typed correctly.
    if (!pendingToken) {
      setError("This sign-in attempt has expired. Go back and sign in again.");
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      await createZitadelSignIn(env.apiBaseUrl).verifyTotp(
        pendingToken,
        entered,
        setTenantId,
      );
      router.replace("/(tabs)");
    } catch (e: unknown) {
      // Mapped from the server's stable code, never the status: 401 covers
      // both a wrong code and an expired challenge, and "sign-in
      // unavailable" must never read as "wrong code" or a merchant will
      // retype a correct one indefinitely.
      const failure = e instanceof ZitadelAuthError ? e.code : "";
      setError(
        failure === "invalid_totp"
          ? // A TOTP code rolls every 30 seconds, so waiting for the next
            // one is real, actionable advice — unlike the email screen,
            // where the only way to a fresh code is signing in again.
            "That code isn't right, or it has expired. Wait for the next code and try again."
          : failure === "auth_unavailable" || failure === "network"
            ? "Sign-in is temporarily unavailable. Try again shortly."
            : "Something went wrong. Please try again.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthScreen>
      <Text preset="eyebrow" className="text-moss">
        TWO-FACTOR AUTHENTICATION
      </Text>
      <Text preset="display" className="mt-2 text-ink">
        Enter code
      </Text>
      <Text preset="body" className="mt-3 text-ink-secondary">
        Open your authenticator app and enter the 6-digit code for Mark8ly.
      </Text>

      <CodeInput
        accessibilityLabel="Authenticator code"
        // Not "email": the delivered-code AutoFill hints cannot apply to a
        // code that is never delivered. See CodeInput's codeSource.
        codeSource="authenticator"
        onChangeText={(next) => {
          setCode(next);
          // Clear a stale "that code isn't right" the moment they start
          // correcting it, rather than leaving the old verdict under a new
          // code.
          if (error) setError(null);
        }}
        onFilled={(next) => void handleVerify(next)}
        disabled={submitting}
      />

      {error ? (
        <Text
          preset="caption"
          className="mt-3 text-danger"
          accessibilityRole="alert"
          accessibilityLiveRegion="polite"
        >
          {error}
        </Text>
      ) : null}

      <Pressable
        accessibilityRole="button"
        accessibilityLabel="Verify code"
        disabled={submitting}
        onPress={() => void handleVerify()}
        className="mt-6 min-h-touch items-center justify-center rounded bg-ink active:opacity-90"
      >
        <Text preset="bodyEmphasis" className="text-paper">
          {submitting ? "Verifying…" : "Verify"}
        </Text>
      </Pressable>

      <View className="mt-6 items-center">
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Back to sign in"
          onPress={() => router.replace("/login")}
        >
          <Text preset="caption" className="text-moss underline">
            Back to sign in
          </Text>
        </Pressable>
      </View>
    </AuthScreen>
  );
}

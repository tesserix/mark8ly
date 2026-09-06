import { useState } from "react";
import {
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  TextInput,
  View,
} from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { router, useLocalSearchParams } from "expo-router";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { createZitadelSignIn } from "@repo/mobile-shared/auth/zitadel-signin";
import { ZitadelAuthError } from "@repo/mobile-shared/auth/zitadel-client";
import { useEnvironment } from "@repo/mobile-shared/config/env";
import { theme } from "@/lib/theme";
import { Text } from "../components/ui/Text";

/**
 * Email one-time code screen (#686).
 *
 * This is the COMMON first sign-in on mobile, not an edge case: a fresh
 * install is by definition an unrecognised device, so a merchant setting up
 * a new phone lands here every time. Its absence is why the equivalent web
 * flow silently bounced users back to /login (#493) — every layer logged
 * success and the code screen simply did not exist.
 */
export default function OtpScreen() {
  const params = useLocalSearchParams<{
    pendingToken?: string;
    email?: string;
  }>();
  const env = useEnvironment();
  const setTenantId = useTenantStore((s) => s.setTenantId);

  const [code, setCode] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const pendingToken = params.pendingToken ?? "";

  async function handleVerify() {
    if (submitting) return;
    if (code.trim().length === 0) {
      setError("Enter the code we emailed you.");
      return;
    }
    // Without this there is nothing to resume, and submitting would fail
    // with copy that blames the code the user typed correctly.
    if (!pendingToken) {
      setError("This sign-in attempt has expired. Go back and sign in again.");
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      await createZitadelSignIn(env.apiBaseUrl).verifyOtp(
        pendingToken,
        code.trim(),
        setTenantId,
      );
      router.replace("/(tabs)");
    } catch (e: unknown) {
      // Mapped from the server's stable code, never the status: 401 covers
      // both a wrong code and an expired challenge, and "sign-in
      // unavailable" must never read as "wrong code" or a merchant will
      // retype a correct one indefinitely.
      const code = e instanceof ZitadelAuthError ? e.code : "";
      setError(
        code === "invalid_code"
          ? // "Request a new one" was the first wording and it was a dead
            // end: this screen has no resend, so the only way to get a fresh
            // code is to sign in again. Advice the UI cannot honour is worse
            // than none. A real resend needs a mobile equivalent of
            // auth-bff's cookie-based /auth/otp/resend — worth adding, since
            // the challenge expires after 5 minutes and reading an email can
            // easily take longer.
            "That code isn't right, or it has expired. Sign in again to get a new code."
          : code === "auth_unavailable" || code === "network"
            ? "Sign-in is temporarily unavailable. Try again shortly."
            : "Something went wrong. Please try again.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <SafeAreaView className="flex-1 bg-paper">
      <KeyboardAvoidingView
        className="flex-1"
        behavior={Platform.OS === "ios" ? "padding" : undefined}
      >
        {/* Mirrors login.tsx exactly — contentContainerStyle flexGrow + an
            inner flex-1 px-6 pt-16 — so the two screens share a top edge and
            a left margin. Centring the content (the first attempt) pushed
            the heading into the middle of a tall device and read as though
            the top of the screen had been cut off. */}
        <ScrollView
          contentContainerStyle={{ flexGrow: 1 }}
          keyboardShouldPersistTaps="handled"
        >
          <View className="flex-1 px-6 pt-16">
            <Text preset="eyebrow" className="text-moss">
              CHECK YOUR EMAIL
            </Text>
            <Text preset="display" className="mt-2 text-ink">
              Enter code
            </Text>
            <Text preset="body" className="mt-3 text-ink-secondary">
              {params.email
                ? `We sent a sign-in code to ${params.email}.`
                : "We sent a sign-in code to your email."}
            </Text>

            <TextInput
              accessibilityLabel="Email code"
              className="mt-8 min-h-touch rounded border border-border bg-paper-elevated px-4 text-center font-sans text-display text-ink"
              placeholder="000000"
              placeholderTextColor={theme.colors.textTertiary}
              keyboardType="number-pad"
              textContentType="oneTimeCode"
              autoComplete="one-time-code"
              maxLength={8}
              value={code}
              onChangeText={setCode}
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
              onPress={handleVerify}
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
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

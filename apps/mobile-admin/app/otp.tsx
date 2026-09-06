import { useEffect, useRef, useState } from "react";
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
 * Seconds the Resend action stays disabled after the screen mounts and
 * after each successful resend.
 *
 * The server allows only a handful of codes per address per 15-minute
 * window and the sign-in itself already spent one, leaving roughly four
 * resends. The cooldown exists so a merchant cannot burn the whole budget
 * in under a minute and then face a wall with no usable code — not to slow
 * down anyone acting reasonably, for whom 30 seconds is about how long it
 * takes to check an inbox.
 */
const RESEND_COOLDOWN_SECONDS = 30;

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
  const [notice, setNotice] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resending, setResending] = useState(false);
  const [cooldown, setCooldown] = useState(RESEND_COOLDOWN_SECONDS);
  /**
   * Bumped on each successful resend, and used as CodeInput's `key`.
   *
   * CodeInput is deliberately uncontrolled (see its props doc), so clearing
   * the parent's `code` alone would leave the old digits visible in cells
   * the screen no longer believes are filled. Remounting is the honest way
   * to clear it: the digits on screen are for the code that just got
   * superseded.
   */
  const [attempt, setAttempt] = useState(0);

  /**
   * STATE, not the route param, and this is load-bearing (#686 item 3).
   *
   * A resend re-seals the challenge and returns a NEW pending token,
   * because the emailed code and the sealed challenge expire together —
   * re-mailing only the code would hand the merchant a correct code
   * against a dead challenge. Reading `params.pendingToken` at verify time
   * would submit the stale half of that pair and fail with copy blaming a
   * code they typed correctly.
   */
  const [pendingToken, setPendingToken] = useState(params.pendingToken ?? "");

  // One interval for the whole screen, cleared on unmount so a resend that
  // finishes after the merchant navigates away cannot setState on a gone
  // component.
  const tick = useRef<ReturnType<typeof setInterval> | null>(null);
  useEffect(() => {
    tick.current = setInterval(() => {
      setCooldown((s) => (s > 0 ? s - 1 : 0));
    }, 1000);
    return () => {
      if (tick.current) clearInterval(tick.current);
      tick.current = null;
    };
  }, []);

  /**
   * `submitted` is the value CodeInput reports the moment its last cell
   * fills. Reading `code` there would read the previous render's state and
   * verify a five-digit code.
   */
  async function handleVerify(submitted?: string) {
    if (submitting) return;
    const entered = (submitted ?? code).trim();
    if (entered.length === 0) {
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
        entered,
        setTenantId,
      );
      router.replace("/(tabs)");
    } catch (e: unknown) {
      // Mapped from the server's stable code, never the status: 401 covers
      // both a wrong code and an expired challenge, and "sign-in
      // unavailable" must never read as "wrong code" or a merchant will
      // retype a correct one indefinitely.
      const code = e instanceof ZitadelAuthError ? e.code : "";
      setNotice(null);
      setError(
        code === "invalid_code"
          ? // Points at the Resend button below, which now exists. The
            // original wording sent people back to sign in again, which is
            // no longer the only way to get a fresh code.
            "That code isn't right, or it has expired. Tap Resend to get a new one."
          : code === "auth_unavailable" || code === "network"
            ? "Sign-in is temporarily unavailable. Try again shortly."
            : "Something went wrong. Please try again.",
      );
    } finally {
      setSubmitting(false);
    }
  }

  async function handleResend() {
    if (resending || cooldown > 0) return;
    if (!pendingToken) {
      setError("This sign-in attempt has expired. Go back and sign in again.");
      return;
    }

    setResending(true);
    setError(null);
    setNotice(null);
    try {
      const fresh = await createZitadelSignIn(env.apiBaseUrl).resendOtp(
        pendingToken,
      );
      // The swap that makes the next verify work. See the pendingToken
      // state declaration above.
      setPendingToken(fresh);
      setCode("");
      setAttempt((n) => n + 1);
      setNotice("We sent a new code.");
      setCooldown(RESEND_COOLDOWN_SECONDS);
    } catch (e: unknown) {
      const code = e instanceof ZitadelAuthError ? e.code : "";
      setError(
        code === "rate_limited"
          ? // Its own copy on purpose: the only thing that helps here is
            // waiting, and "try again" would send them round a loop that
            // cannot end until the window rolls.
            "You've requested too many codes. Try again in a few minutes."
          : code === "invalid_challenge"
            ? "This sign-in attempt has expired. Go back and sign in again."
            : code === "auth_unavailable" || code === "network"
              ? "Sign-in is temporarily unavailable. Try again shortly."
              : "We couldn't send a new code. Please try again.",
      );
    } finally {
      setResending(false);
    }
  }

  const resendDisabled = resending || cooldown > 0;

  return (
    <AuthScreen>
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

      <CodeInput
        key={attempt}
        accessibilityLabel="Email code"
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

      {!error && notice ? (
        <Text
          preset="caption"
          className="mt-3 text-moss"
          accessibilityLiveRegion="polite"
        >
          {notice}
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
        <Text preset="caption" className="text-ink-secondary">
          Didn&apos;t receive the code?
        </Text>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Resend code"
          accessibilityState={{ disabled: resendDisabled }}
          disabled={resendDisabled}
          onPress={() => void handleResend()}
          className="mt-2 min-h-touch items-center justify-center px-2"
        >
          <Text
            preset="caption"
            className={
              resendDisabled ? "text-ink-muted" : "text-moss underline"
            }
          >
            {resending
              ? "Sending…"
              : cooldown > 0
                ? `Resend in ${cooldown}s`
                : "Resend code"}
          </Text>
        </Pressable>
      </View>

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

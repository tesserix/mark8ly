import { useEffect, useState } from 'react';
import {
  Linking,
  Platform,
  Pressable,
  StyleSheet,
  TextInput,
  View,
} from 'react-native';
import Svg, { Path } from 'react-native-svg';
import { authErrorMessage } from '@repo/mobile-shared/auth/errors';
import { useAuthNoticeStore, type AuthNotice } from '@repo/mobile-shared/stores/auth-notice';
import { theme } from '@/lib/theme';
import { AuthScreen } from '@/components/ui/AuthScreen';
import { IconButton } from '@/components/ui/IconButton';
import { Text } from '../components/ui/Text';
import { router } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { useTenantStore } from '@repo/mobile-shared/stores/tenant-store';
import { useEnvironment } from '@repo/mobile-shared/config/env';
import { createZitadelSignIn, type SignInOutcome } from '@repo/mobile-shared/auth/zitadel-signin';
import { ZitadelAuthError, type IdpProvider } from '@repo/mobile-shared/auth/zitadel-client';

// Shown when an error can't be mapped to specific copy, so a sign-in failure
// never leaves the form silent. `authErrorMessage` returns null ONLY for a
// user-cancelled sheet — that case stays silent by design.
const GENERIC_AUTH_ERROR = 'Something went wrong. Please try again.';

// Both stores require an in-app, reachable privacy policy for account-based
// apps. These point at the live web pages served from mark8ly.com.
const PRIVACY_URL = 'https://mark8ly.com/privacy';
const TERMS_URL = 'https://mark8ly.com/terms';

/**
 * Where the web bridge page (apps/admin/app/auth/idp/mobile/route.ts) sends
 * the browser at the end of a Zitadel Google sign-in. `ASWebAuthenticationSession`
 * matches on this scheme and closes, handing the URL back.
 *
 * It cannot be the Zitadel return URL directly: auth-bff's allowlist
 * requires https on an allowlisted host, and that check is the ONLY thing
 * stopping a completed admin sign-in being handed to another origin. The
 * scheme itself is registered in app.config.js — the two must match.
 */
const IDP_REDIRECT_URL = 'mark8ly-admin://auth/idp';

const NOTICE_COPY: Record<AuthNotice, string> = {
  'access-denied': "That account doesn't have access to a Mark8ly admin account.",
  'no-session': 'Your session ended. Sign in again.',
};

/**
 * Provider target box. 56 clears the 44pt minimum as a REAL sized box (see
 * IconButton) and clears Google's own 40dp floor for the square icon-only
 * "G" button with headroom. Both providers use the same number so neither
 * can end up less prominent than the other — Apple's logo-only button is
 * only permitted when it is no LESS prominent than the alternatives.
 *
 * There is no text inside either target, so the fixed height cannot clip at
 * a raised Dynamic Type setting — the four silent-clipping bugs earlier in
 * this programme were all fixed heights wrapping *text*.
 */
const PROVIDER_BOX = 56;
/** Mark size inside the box: comfortably above Google's 18dp logo floor. */
const PROVIDER_MARK = 24;

/**
 * Google's official four-colour "G", inlined because lucide dropped its
 * brand marks and the constraint is zero new dependencies. Geometry and
 * fills are Google's published mark verbatim on a 24×24 viewBox — Google's
 * guidelines forbid recolouring it, so these four hexes are exempt from the
 * one-accent rule as a brand mark rather than decoration and must not be
 * mapped onto Paper/Ink/Moss.
 */
function GoogleMark({ size = PROVIDER_MARK }: { size?: number }) {
  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" testID="google-mark">
      <Path
        fill="#4285F4"
        d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
      />
      <Path
        fill="#34A853"
        d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
      />
      <Path
        fill="#FBBC05"
        d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"
      />
      <Path
        fill="#EA4335"
        d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
      />
    </Svg>
  );
}

/**
 * The Apple mark, monochrome as Apple requires, drawn in Ink on our elevated
 * Paper surface — the official WHITE-WITH-BLACK-OUTLINE appearance, one of the
 * three Apple publishes (black / white / white-with-outline).
 *
 * We used to render the BLACK appearance, a solid ink fill. It was compliant,
 * but it put a second solid on a page whose only solid is meant to be the
 * full-width ink "Sign in" bar, and it left the two provider targets visibly
 * mismatched — an outlined white box beside a filled black one. The outline
 * appearance is what lets both targets be the same Paper surface with the same
 * hairline, which is the actual design intent this row was built for.
 *
 * Ink on Paper is 17.4:1. The hairline is `border` (bone), NOT the black Apple
 * draws in its own artwork — the mark itself is unaltered, which is what Apple
 * constrains; the surrounding button is ours to style, and every other target
 * in this app uses this same hairline.
 */
function AppleMark({ size = PROVIDER_MARK }: { size?: number }) {
  return (
    <Svg width={size} height={size} viewBox="0 0 24 24" testID="apple-mark">
      <Path
        fill={theme.colors.text}
        d="M12.152 6.896c-.948 0-2.415-1.078-3.96-1.04-2.04.027-3.91 1.183-4.961 3.014-2.117 3.675-.546 9.103 1.519 12.09 1.013 1.454 2.208 3.09 3.792 3.039 1.52-.065 2.09-.987 3.935-.987 1.831 0 2.35.987 3.96.948 1.637-.026 2.676-1.48 3.676-2.948 1.156-1.688 1.636-3.325 1.662-3.415-.039-.013-3.182-1.221-3.22-4.857-.026-3.04 2.48-4.494 2.597-4.559-1.416-2.09-3.623-2.324-4.39-2.376-2-.156-3.675 1.088-4.61 1.088zM15.53 3.83c.843-1.012 1.4-2.427 1.245-3.83-1.207.052-2.662.805-3.532 1.818-.78.896-1.454 2.338-1.273 3.714 1.338.104 2.715-.688 3.559-1.701"
      />
    </Svg>
  );
}

export default function LoginScreen() {
  const env = useEnvironment();
  const setTenantId = useTenantStore((s) => s.setTenantId);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const notice = useAuthNoticeStore((s) => s.notice);
  const clearNotice = useAuthNoticeStore((s) => s.clearNotice);

  useEffect(() => {
    if (!notice) return;
    setError(NOTICE_COPY[notice]);
    clearNotice();
  }, [notice, clearNotice]);

  async function handleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      // Our own form, posted to marketplace-api rather than to Zitadel's
      // hosted login. A fresh install is always an unrecognised device,
      // so the usual answer here is a challenge, not a session.
      const out = await createZitadelSignIn(env.apiBaseUrl).signIn(email, password, setTenantId);
      if (routeStepUp(out)) return;
      router.replace('/(tabs)');
    } catch (e: unknown) {
      if (e instanceof ZitadelAuthError) {
        // Mapped from the server's stable code. `no_store` is a real,
        // actionable state — the account exists but has no store — and
        // `auth_unavailable` must never read as a wrong password.
        const msg = zitadelErrorMessage(e, null);
        if (msg !== null) setError(msg);
        return;
      }
      const msg = authErrorMessage(e);
      // null is the mapper's deliberate "user cancelled — stay silent" signal;
      // anything else always surfaces copy, falling back to generic wording so
      // an unmapped error never silently shows nothing.
      if (msg !== null) setError(msg || GENERIC_AUTH_ERROR);
    } finally {
      setSubmitting(false);
    }
  }

  /**
   * Sends an outstanding step-up to its screen, and reports whether it did.
   *
   * The two challenges get different screens because they are different
   * things: /otp says "check your email", which is wrong — actively
   * misleading — for a code that only ever exists inside an authenticator
   * app. Shared by the password and Google paths so the same challenge
   * cannot land on two different screens depending on how the merchant
   * signed in.
   */
  function routeStepUp(out: SignInOutcome): boolean {
    if (out.kind !== 'otp' && out.kind !== 'totp') return false;
    router.push({
      pathname: out.kind === 'totp' ? '/totp' : '/otp',
      // The TOTP screen shows no address — the code is not sent anywhere —
      // so it is given only what it needs to resume.
      params:
        out.kind === 'totp'
          ? { pendingToken: out.pendingToken ?? '' }
          : { pendingToken: out.pendingToken ?? '', email: out.email },
    });
    return true;
  }

  /**
   * Maps a ZitadelAuthError to copy. Shared by the password, Google and
   * Apple paths so they cannot answer the same server code differently —
   * `auth_unavailable` in particular must never read as a wrong
   * credential, or a merchant retries a correct one indefinitely.
   *
   * `attempted` is which button was pressed, and null for the password
   * form. It exists because two of these strings NAME a provider, and the
   * screen renders this mapping rather than the server's own message
   * (nothing here shows `e.message`) — so a merchant who tapped Apple and
   * read "Couldn't sign you in with Google" would be told the wrong thing
   * about which sign-in failed (#771).
   *
   * Returns null for a deliberate cancellation, matching
   * `authErrorMessage`'s own "stay silent" contract.
   */
  function zitadelErrorMessage(e: ZitadelAuthError, attempted: IdpProvider | null): string | null {
    const providerName = attempted === 'apple' ? 'Apple' : 'Google';
    switch (e.code) {
      case 'cancelled':
        return null;
      case 'invalid_credentials':
        return "Couldn't sign you in. Check your details and try again.";
      case 'no_store':
        return "We couldn't find a store for this account. Did you finish onboarding?";
      case 'email_not_verified':
        // Only ever reachable on the federated path — auth-bff raises it
        // from idp/finish, never from a password login — so naming the
        // provider that failed to verify is accurate here.
        return attempted === null
          ? "That email address isn't verified, so we can't sign you in with it."
          : `${providerName} hasn't verified that email address, so we can't sign you in with it.`;
      case 'email_ambiguous':
        return 'More than one account uses that email. Contact support to sort it out.';
      case 'google_sign_in_failed':
        return "Couldn't sign you in with Google. Try again.";
      case 'apple_sign_in_failed':
        return "Couldn't sign you in with Apple. Try again.";
      case 'unsupported_provider':
        return "That sign-in method isn't available.";
      case 'auth_unavailable':
      case 'network':
        return 'Sign-in is temporarily unavailable. Try again shortly.';
      case 'challenge_unresumable':
        // The server asked for a verification step but sent nothing to
        // resume it with — in practice, an app newer than the API it is
        // talking to. Distinct copy because "try again" is useless advice
        // here: retrying repeats it.
        //
        // A TOTP enrolment used to land here on EVERY sign-in (#686 item
        // 2): the login answered totp_required with no pending_token at
        // all, so a merchant with an authenticator was told to update an
        // app that was already current. The server now seals one, and this
        // stays for its real case.
        return 'This app version needs an update to finish signing in.';
      default:
        return GENERIC_AUTH_ERROR;
    }
  }

  async function handleGoogleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      // Zitadel's IDP-intent flow: the browser round trip is what proves
      // the Google identity, and the app never sees a Google token at all.
      // Mirrors handleSignIn above, including routing a step-up to the
      // same /otp screen.
      const out = await createZitadelSignIn(env.apiBaseUrl).signInWithGoogle(
        {
          redirectUrl: IDP_REDIRECT_URL,
          openAuthSession: (authUrl, redirectUrl) =>
            WebBrowser.openAuthSessionAsync(authUrl, redirectUrl),
        },
        setTenantId,
      );
      if (routeStepUp(out)) return;
      router.replace('/(tabs)');
    } catch (e: unknown) {
      if (e instanceof ZitadelAuthError) {
        const msg = zitadelErrorMessage(e, 'google');
        if (msg !== null) setError(msg);
        return;
      }
      const msg = authErrorMessage(e);
      // null is the mapper's deliberate "user cancelled — stay silent" signal;
      // anything else always surfaces copy, falling back to generic wording so
      // an unmapped error never silently shows nothing.
      if (msg !== null) setError(msg || GENERIC_AUTH_ERROR);
    } finally {
      setSubmitting(false);
    }
  }

  async function handleAppleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      // Zitadel's IDP-intent flow, not expo-apple-authentication: the
      // browser round trip is what proves the Apple identity, and the app
      // never sees an Apple token at all. Mirrors handleGoogleSignIn,
      // including routing a step-up to the same screens (#771).
      const out = await createZitadelSignIn(env.apiBaseUrl).signInWithApple(
        {
          redirectUrl: IDP_REDIRECT_URL,
          openAuthSession: (authUrl, redirectUrl) =>
            WebBrowser.openAuthSessionAsync(authUrl, redirectUrl),
        },
        setTenantId,
      );
      if (routeStepUp(out)) return;
      router.replace('/(tabs)');
    } catch (e: unknown) {
      if (e instanceof ZitadelAuthError) {
        const msg = zitadelErrorMessage(e, 'apple');
        if (msg !== null) setError(msg);
        return;
      }
      const msg = authErrorMessage(e);
      // null is the mapper's deliberate "user cancelled — stay silent" signal;
      // anything else always surfaces copy, falling back to generic wording so
      // an unmapped error never silently shows nothing.
      if (msg !== null) setError(msg || GENERIC_AUTH_ERROR);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthScreen>
      <Text preset="eyebrow" className="text-moss">
        Merchant admin
      </Text>
      <Text preset="display" className="mt-2">
        Mark8ly
      </Text>
      <Text preset="body" className="mt-2 text-ink-muted">
        Sign in to manage your store.
      </Text>

      <View className="mt-8 gap-3">
        <TextInput
          accessibilityLabel="Email"
          className="min-h-touch rounded border border-border bg-paper-elevated px-4 font-sans text-body text-ink"
          placeholder="Email"
          placeholderTextColor={theme.colors.textTertiary}
          autoCapitalize="none"
          keyboardType="email-address"
          textContentType="emailAddress"
          autoComplete="email"
          value={email}
          onChangeText={setEmail}
        />
        <TextInput
          accessibilityLabel="Password"
          className="min-h-touch rounded border border-border bg-paper-elevated px-4 font-sans text-body text-ink"
          placeholder="Password"
          placeholderTextColor={theme.colors.textTertiary}
          secureTextEntry
          textContentType="password"
          autoComplete="password"
          value={password}
          onChangeText={setPassword}
        />
      </View>

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
        accessibilityLabel="Sign in"
        disabled={submitting}
        onPress={handleSignIn}
        className="mt-6 min-h-touch items-center justify-center rounded bg-ink active:opacity-90"
      >
        <Text preset="bodyEmphasis" className="text-paper">
          {submitting ? 'Signing in…' : 'Sign in'}
        </Text>
      </Pressable>

      <View className="mt-6 flex-row items-center gap-3">
        <View className="h-px flex-1 bg-border" />
        <Text preset="caption">or</Text>
        <View className="h-px flex-1 bg-border" />
      </View>

      {/*
        Deliberately CENTRED — the one local exception to the page's
        left-aligned, asymmetric rhythm. A provider row is a pair of
        equal-weight alternatives with no reading order, not a hero;
        everything around it stays left-aligned. Collapsing both
        providers to icons leaves the full-width ink "Sign in" bar as
        the only solid on screen, which is the whole point of the
        change.
      */}
      <View style={styles.providerRow} testID="provider-row">
        <IconButton
          accessibilityLabel="Continue with Google"
          disabled={submitting}
          onPress={handleGoogleSignIn}
          testID="provider-google"
          // "ink" tone: the Google box is an outline/white surface, so
          // it takes the dark ripple and the standard iOS press dim.
          tone="ink"
          style={[styles.providerBox, styles.providerBoxOutline]}
        >
          <GoogleMark />
        </IconButton>

        {Platform.OS === 'ios' ? (
          <IconButton
            accessibilityLabel="Sign in with Apple"
            disabled={submitting}
            onPress={handleAppleSignIn}
            testID="provider-apple"
            // "ink" tone, matching Google: both targets are now the same
            // outline/Paper surface, so both take the dark ripple and the
            // standard iOS press dim. (The "onDark" tone exists for solid
            // fills, where a 45% fade reads as broken rather than pressed
            // — see lib/theme.ts. Nothing on this row is solid any more.)
            tone="ink"
            style={[styles.providerBox, styles.providerBoxOutline]}
          >
            <AppleMark />
          </IconButton>
        ) : null}
      </View>

      <View className="mt-8 flex-row items-center justify-center gap-3">
        <Pressable
          accessibilityRole="link"
          accessibilityLabel="Privacy Policy"
          onPress={() => Linking.openURL(PRIVACY_URL)}
        >
          <Text preset="caption" className="underline">
            Privacy Policy
          </Text>
        </Pressable>
        <Text preset="caption">·</Text>
        <Pressable
          accessibilityRole="link"
          accessibilityLabel="Terms of Service"
          onPress={() => Linking.openURL(TERMS_URL)}
        >
          <Text preset="caption" className="underline">
            Terms of Service
          </Text>
        </Pressable>
      </View>
    </AuthScreen>
  );
}

const styles = StyleSheet.create({
  /**
   * Expressed here rather than as `className="… gap-4"` on purpose. `gap-4`
   * appeared nowhere else in this app, and Tailwind's JIT only emits the
   * utilities it finds in already-scanned content — the first device render
   * came back with the two boxes FLUSH (measured 0px between them in
   * /tmp/task10/05-login.png) while every unit test stayed green, because
   * RNTL renders without the NativeWind runtime. Same silent-drop family as
   * the function-style-prop landmine. A StyleSheet value cannot be dropped.
   */
  providerRow: {
    marginTop: theme.spacing.xxl,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: theme.spacing.lg,
  },
  // Shared by BOTH providers so neither can drift smaller than the other.
  providerBox: {
    width: PROVIDER_BOX,
    height: PROVIDER_BOX,
    borderRadius: theme.radius,
  },
  providerBoxOutline: {
    backgroundColor: theme.colors.elevated,
    borderWidth: 1,
    borderColor: theme.colors.border,
  },
});

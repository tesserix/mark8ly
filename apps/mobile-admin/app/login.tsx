import { useEffect, useState } from 'react';
import { KeyboardAvoidingView, Platform, Pressable, ScrollView, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useAuth } from '@repo/mobile-shared/auth/provider';
import { authErrorMessage } from '@repo/mobile-shared/auth/errors';
import type { SocialSignInOutcome } from '@repo/mobile-shared/auth/social-credentials';
import { useAuthNoticeStore, type AuthNotice } from '@repo/mobile-shared/stores/auth-notice';
import { configureGoogleSignin, signInWithAppleNative, signInWithGoogleNative } from '@/lib/social-auth';
import { theme } from '@/lib/theme';
import { LinkAccountPrompt } from '../components/auth/LinkAccountPrompt';
import { Text } from '../components/ui/Text';

const DEMO_AUTH = process.env.EXPO_PUBLIC_AUTH_BACKEND === 'demo';

type LinkTarget = Extract<SocialSignInOutcome, { status: 'needs-link' }>;

const NOTICE_COPY: Record<AuthNotice, string> = {
  'access-denied': "That account doesn't have access to a Mark8ly admin account.",
  'no-session': 'Your session ended. Sign in again.',
};

export default function LoginScreen() {
  const { signIn, signInWithGoogle, signInWithApple } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [linkTarget, setLinkTarget] = useState<LinkTarget | null>(null);
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
      await signIn(email, password);
    } catch (e: unknown) {
      const msg = authErrorMessage(e);
      if (msg) setError(msg);
    } finally {
      setSubmitting(false);
    }
  }

  async function handleGoogleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      let outcome: SocialSignInOutcome;
      if (DEMO_AUTH) {
        outcome = await signInWithGoogle('demo-google-token');
      } else {
        configureGoogleSignin();
        const idToken = await signInWithGoogleNative();
        outcome = await signInWithGoogle(idToken);
      }
      if (outcome.status === 'needs-link') setLinkTarget(outcome);
    } catch (e: unknown) {
      const msg = authErrorMessage(e);
      if (msg) setError(msg);
    } finally {
      setSubmitting(false);
    }
  }

  async function handleAppleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      let outcome: SocialSignInOutcome;
      if (DEMO_AUTH) {
        outcome = await signInWithApple('demo-apple-token', '', null);
      } else {
        const { idToken, rawNonce, fullName } = await signInWithAppleNative();
        outcome = await signInWithApple(idToken, rawNonce, fullName);
      }
      if (outcome.status === 'needs-link') setLinkTarget(outcome);
    } catch (e: unknown) {
      const msg = authErrorMessage(e);
      if (msg) setError(msg);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <SafeAreaView className="flex-1 bg-paper">
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : 'height'}>
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} keyboardShouldPersistTaps="handled">
          <View className="flex-1 px-6 pt-16">
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

            <Pressable
              accessibilityRole="button"
              accessibilityLabel="Continue with Google"
              disabled={submitting}
              onPress={handleGoogleSignIn}
              className="mt-6 min-h-touch items-center justify-center rounded border border-border bg-paper-elevated active:opacity-90"
            >
              <Text preset="bodyEmphasis">Continue with Google</Text>
            </Pressable>

            <Pressable
              accessibilityRole="button"
              accessibilityLabel="Sign in with Apple"
              disabled={submitting}
              onPress={handleAppleSignIn}
              className="mt-3 min-h-touch items-center justify-center rounded bg-ink active:opacity-90"
            >
              <Text preset="bodyEmphasis" className="text-paper">
                Sign in with Apple
              </Text>
            </Pressable>

            {linkTarget ? (
              <LinkAccountPrompt
                visible
                email={linkTarget.email}
                provider={linkTarget.provider}
                pendingCredential={linkTarget.pendingCredential}
                onCancel={() => setLinkTarget(null)}
                onLinked={() => setLinkTarget(null)}
              />
            ) : null}
          </View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

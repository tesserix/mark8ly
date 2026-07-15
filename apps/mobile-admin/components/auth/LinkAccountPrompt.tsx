import { useEffect, useState } from 'react';
import { Modal, Pressable, TextInput, View } from 'react-native';
import type { FirebaseAuthTypes } from '@react-native-firebase/auth';
import { useAuth } from '@repo/mobile-shared/auth/provider';
import { authErrorMessage, type AuthErrorContext } from '@repo/mobile-shared/auth/errors';
import {
  configureGoogleSignin,
  signInWithAppleNative,
  signInWithGoogleNative,
} from '@/lib/social-auth';
import { Text } from '../ui/Text';

export interface LinkAccountPromptProps {
  visible: boolean;
  email: string;
  provider: 'google.com' | 'apple.com';
  pendingCredential: FirebaseAuthTypes.AuthCredential;
  onCancel: () => void;
  onLinked: () => void;
}

const PROVIDER_LABEL: Record<LinkAccountPromptProps['provider'], string> = {
  'google.com': 'Google',
  'apple.com': 'Apple',
};

export function LinkAccountPrompt({
  visible,
  email,
  provider,
  pendingCredential,
  onCancel,
  onLinked,
}: LinkAccountPromptProps) {
  const {
    existingSignInMethods,
    completeLinkWithPassword,
    completeLinkWithGoogle,
    completeLinkWithApple,
  } = useAuth();
  const [methods, setMethods] = useState<string[] | null>(null);
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const found = await existingSignInMethods(email);
        if (!cancelled) setMethods(found);
      } catch {
        // Fail open — unknown methods render every option.
        if (!cancelled) setMethods([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [email, existingSignInMethods]);

  const passwordMatches = methods?.includes('password') ?? false;
  const googleMatches = methods?.includes('google.com') ?? false;
  const appleMatches = methods?.includes('apple.com') ?? false;
  // `unknown` covers every case where none of the above would actually
  // render a control — enumeration protection (`[]`), an unrecognized
  // method list (e.g. `["emailLink"]`), and the case where the only
  // matched method is the provider currently being linked (which can
  // never be offered as its own re-auth option). Fail open in all of
  // them: offer password plus every other provider so the sheet never
  // dead-ends with just Cancel.
  const anyControlWouldRender =
    passwordMatches ||
    (provider !== 'google.com' && googleMatches) ||
    (provider !== 'apple.com' && appleMatches);
  const unknown = methods !== null && !anyControlWouldRender;
  const showPassword = methods === null || unknown || passwordMatches;
  const showGoogle = provider !== 'google.com' && (unknown || googleMatches);
  const showApple = provider !== 'apple.com' && (unknown || appleMatches);

  async function run(fn: () => Promise<void>, ctx?: AuthErrorContext) {
    if (busy) return;
    setError(null);
    setBusy(true);
    try {
      await fn();
      onLinked();
    } catch (e: unknown) {
      const msg = authErrorMessage(e, ctx);
      if (msg) setError(msg);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onCancel}>
      <View className="flex-1 justify-end bg-ink/40">
        <View className="rounded-t-xl bg-paper-elevated px-6 pb-10 pt-6">
          <Text preset="h3">Link your account</Text>
          <Text preset="body" className="mt-2 text-ink-muted">
            An account already exists for {email}. Sign in to connect{' '}
            {PROVIDER_LABEL[provider]}.
          </Text>

          {showPassword ? (
            <View className="mt-6 gap-3">
              <TextInput
                accessibilityLabel="Password"
                className="min-h-touch rounded border border-border bg-paper px-4 font-sans text-body text-ink"
                placeholder="Password"
                placeholderTextColor="#7A766E"
                secureTextEntry
                value={password}
                onChangeText={setPassword}
              />
              <Pressable
                accessibilityRole="button"
                accessibilityLabel="Sign in and link"
                disabled={busy}
                onPress={() =>
                  void run(() => completeLinkWithPassword(email, password, pendingCredential))
                }
                className="min-h-touch items-center justify-center rounded bg-ink active:opacity-90"
              >
                <Text preset="bodyEmphasis" className="text-paper">
                  {busy ? 'Linking…' : 'Sign in and link'}
                </Text>
              </Pressable>
            </View>
          ) : null}

          {showGoogle ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="Continue with Google to link"
              disabled={busy}
              onPress={() =>
                void run(async () => {
                  configureGoogleSignin();
                  const idToken = await signInWithGoogleNative();
                  await completeLinkWithGoogle(idToken, pendingCredential);
                }, { provider: 'google.com' })
              }
              className="mt-3 min-h-touch items-center justify-center rounded border border-border bg-paper active:opacity-90"
            >
              <Text preset="bodyEmphasis">Continue with Google to link</Text>
            </Pressable>
          ) : null}

          {showApple ? (
            <Pressable
              accessibilityRole="button"
              accessibilityLabel="Continue with Apple to link"
              disabled={busy}
              onPress={() =>
                void run(async () => {
                  const { idToken, rawNonce } = await signInWithAppleNative();
                  await completeLinkWithApple(idToken, rawNonce, pendingCredential);
                }, { provider: 'apple.com' })
              }
              className="mt-3 min-h-touch items-center justify-center rounded bg-ink active:opacity-90"
            >
              <Text preset="bodyEmphasis" className="text-paper">
                Continue with Apple to link
              </Text>
            </Pressable>
          ) : null}

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
            accessibilityLabel="Cancel"
            disabled={busy}
            onPress={onCancel}
            className="mt-4 min-h-touch items-center justify-center rounded"
          >
            <Text preset="body" className="text-ink-muted">
              Cancel
            </Text>
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

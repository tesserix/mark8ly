import { useEffect, useState } from 'react';
import { Modal, Pressable, TextInput, View } from 'react-native';
import type { FirebaseAuthTypes } from '@react-native-firebase/auth';
import { useAuth } from '@repo/mobile-shared/auth/provider';
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

function getErrorMessage(e: unknown): string {
  return e instanceof Error && e.message
    ? e.message
    : 'Could not link your account. Try again.';
}

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

  // `[]` means enumeration protection hid the answer — offer everything.
  const unknown = methods !== null && methods.length === 0;
  const showPassword = methods === null || unknown || methods.includes('password');
  const showGoogle =
    provider !== 'google.com' && (unknown || (methods?.includes('google.com') ?? false));
  const showApple =
    provider !== 'apple.com' && (unknown || (methods?.includes('apple.com') ?? false));

  async function run(fn: () => Promise<void>) {
    if (busy) return;
    setError(null);
    setBusy(true);
    try {
      await fn();
      onLinked();
    } catch (e: unknown) {
      setError(getErrorMessage(e));
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
                })
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
                })
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

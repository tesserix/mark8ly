import { useState } from 'react';
import { Pressable, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useAuth } from '@repo/mobile-shared/auth/provider';
import { configureGoogleSignin, signInWithAppleNative, signInWithGoogleNative } from '@/lib/social-auth';
import { Text } from '../components/ui/Text';

const DEMO_AUTH = process.env.EXPO_PUBLIC_AUTH_BACKEND === 'demo';

function getErrorMessage(e: unknown): string {
  return e instanceof Error && e.message
    ? e.message
    : 'Could not sign in. Check your details and try again.';
}

export default function LoginScreen() {
  const { signIn, signInWithGoogle, signInWithApple } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      await signIn(email, password);
    } catch (e: unknown) {
      setError(getErrorMessage(e));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleGoogleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      if (DEMO_AUTH) {
        await signInWithGoogle('demo-google-token');
      } else {
        configureGoogleSignin();
        const idToken = await signInWithGoogleNative();
        await signInWithGoogle(idToken);
      }
    } catch (e: unknown) {
      setError(getErrorMessage(e));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleAppleSignIn() {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      if (DEMO_AUTH) {
        await signInWithApple('demo-apple-token', '', null);
      } else {
        const { idToken, rawNonce, fullName } = await signInWithAppleNative();
        await signInWithApple(idToken, rawNonce, fullName);
      }
    } catch (e: unknown) {
      setError(getErrorMessage(e));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <SafeAreaView className="flex-1 bg-paper">
      <View className="flex-1 justify-center px-6">
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
            placeholderTextColor="#7A766E"
            autoCapitalize="none"
            keyboardType="email-address"
            value={email}
            onChangeText={setEmail}
          />
          <TextInput
            accessibilityLabel="Password"
            className="min-h-touch rounded border border-border bg-paper-elevated px-4 font-sans text-body text-ink"
            placeholder="Password"
            placeholderTextColor="#7A766E"
            secureTextEntry
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
      </View>
    </SafeAreaView>
  );
}

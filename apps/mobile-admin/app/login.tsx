import { useState } from 'react';
import { Pressable, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useAuth } from '@repo/mobile-shared/auth/provider';
import { Text } from '../components/ui/Text';

export default function LoginScreen() {
  const { signIn, loading } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

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

        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Sign in"
          disabled={loading}
          onPress={() => signIn(email, password)}
          className="mt-6 min-h-touch items-center justify-center rounded bg-ink active:opacity-90"
        >
          <Text preset="bodyEmphasis" className="text-paper">
            {loading ? 'Signing in…' : 'Sign in'}
          </Text>
        </Pressable>
      </View>
    </SafeAreaView>
  );
}

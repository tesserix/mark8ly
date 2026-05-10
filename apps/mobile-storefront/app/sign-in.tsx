import { useState } from "react";
import {
  KeyboardAvoidingView,
  Platform,
  StyleSheet,
  TextInput,
  TouchableOpacity,
  View,
} from "react-native";
import { useRouter, Stack } from "expo-router";
import { ChevronLeft } from "lucide-react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { Button, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { getMerchant } from "@/lib/merchant";

export default function SignInScreen() {
  const router = useRouter();
  const merchant = getMerchant();
  const { signIn } = useAuth();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSignIn = async () => {
    if (!email.trim() || !password) {
      setError("Email and password are required");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      await signIn(email.trim(), password);
      router.replace("/(tabs)/account");
    } catch (err) {
      const message = err instanceof Error ? err.message : "Sign-in failed";
      if (message.includes("wrong-password") || message.includes("user-not-found")) {
        setError("Invalid email or password");
      } else if (message.includes("network")) {
        setError("Network error — check your connection");
      } else {
        setError("Sign-in failed — please try again");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: false }} />
      <KeyboardAvoidingView
        style={{ flex: 1 }}
        behavior={Platform.OS === "ios" ? "padding" : undefined}
      >
        <View style={styles.headerBar}>
          <TouchableOpacity
            onPress={() => router.back()}
            hitSlop={12}
            accessibilityRole="button"
            accessibilityLabel="Back"
            style={styles.backBtn}
          >
            <ChevronLeft size={22} color={theme.colors.text} strokeWidth={1.75} />
          </TouchableOpacity>
        </View>

        <View style={styles.body}>
          <View style={styles.brand}>
            <Text preset="eyebrow" color="textTertiary">
              {merchant.shortName.toUpperCase()}
            </Text>
            <Text preset="display" color="text" style={styles.wordmark}>
              Welcome back
            </Text>
            <Text preset="bodyLg" color="textSecondary">
              Sign in to your account.
            </Text>
          </View>

          {error ? (
            <View style={styles.errorBox}>
              <Text preset="caption" color="danger">
                {error}
              </Text>
            </View>
          ) : null}

          <View style={styles.form}>
            <Field label="Email">
              <TextInput
                placeholder="you@example.com"
                value={email}
                onChangeText={setEmail}
                autoCapitalize="none"
                keyboardType="email-address"
                autoComplete="email"
                style={styles.input}
                placeholderTextColor={theme.colors.textTertiary}
              />
            </Field>
            <Field label="Password">
              <TextInput
                placeholder="••••••••"
                value={password}
                onChangeText={setPassword}
                secureTextEntry
                autoComplete="password"
                style={styles.input}
                placeholderTextColor={theme.colors.textTertiary}
              />
            </Field>

            <Button
              label="Sign in"
              onPress={handleSignIn}
              loading={loading}
              fullWidth
              style={{ marginTop: theme.spacing.lg }}
            />

            <TouchableOpacity
              onPress={() => router.push("/sign-up")}
              style={styles.linkBtn}
              accessibilityRole="link"
              accessibilityLabel="Create an account"
            >
              <Text preset="caption" color="textSecondary">
                New here? <Text preset="caption" color="text">Create an account</Text>
              </Text>
            </TouchableOpacity>
          </View>
        </View>
      </KeyboardAvoidingView>
    </Screen>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <View style={{ gap: theme.spacing.xs }}>
      <Text preset="caption" color="textSecondary" style={{ letterSpacing: 0.4 }}>
        {label}
      </Text>
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  headerBar: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.sm,
  },
  backBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radii.pill,
  },
  body: {
    flex: 1,
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xxxl,
    paddingBottom: theme.spacing.xxl,
  },
  brand: {
    gap: theme.spacing.xs,
    marginBottom: theme.spacing.xxxl,
  },
  wordmark: { marginTop: theme.spacing.sm },
  errorBox: {
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.md,
    borderLeftWidth: 2,
    borderLeftColor: theme.colors.danger,
    marginBottom: theme.spacing.lg,
    backgroundColor: theme.colors.surfaceAlt,
  },
  form: { gap: theme.spacing.lg },
  input: {
    height: 48,
    fontFamily: theme.fonts.sans,
    fontSize: 16,
    color: theme.colors.text,
    borderBottomWidth: 1,
    borderBottomColor: theme.colors.border,
    paddingHorizontal: 0,
  },
  linkBtn: {
    alignSelf: "center",
    marginTop: theme.spacing.md,
    minHeight: 44,
    justifyContent: "center",
    paddingHorizontal: theme.spacing.md,
  },
});

import { useState } from "react";
import {
  View,
  TextInput,
  TouchableOpacity,
  StyleSheet,
  KeyboardAvoidingView,
  Platform,
  ActivityIndicator,
} from "react-native";
import * as Linking from "expo-linking";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useEnvironment } from "@repo/mobile-shared/config/env";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";

export default function LoginScreen() {
  const { signIn } = useAuth();
  const env = useEnvironment();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleLogin = async () => {
    if (!email.trim() || !password) {
      setError("Email and password are required");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      await signIn(email.trim(), password);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Login failed";
      if (message.includes("wrong-password") || message.includes("user-not-found")) {
        setError("Invalid email or password");
      } else if (message.includes("network")) {
        setError("Network error — check your connection");
      } else {
        setError("Login failed — please try again");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === "ios" ? "padding" : undefined}
    >
      <View style={styles.frame}>
        <View style={styles.brand}>
          <Text preset="eyebrow" color="textTertiary">
            MARK8LY ADMIN
          </Text>
          <Text preset="display" color="text" style={styles.wordmark}>
            Welcome back
          </Text>
          <Text preset="bodyLg" color="textSecondary">
            Sign in to your store dashboard.
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
          <View style={styles.field}>
            <Text preset="caption" color="textSecondary" style={styles.label}>
              Email
            </Text>
            <TextInput
              placeholder="you@store.com"
              value={email}
              onChangeText={setEmail}
              autoCapitalize="none"
              keyboardType="email-address"
              autoComplete="email"
              style={styles.input}
              placeholderTextColor={theme.colors.textTertiary}
              accessibilityLabel="Email address"
            />
          </View>

          <View style={styles.field}>
            <Text preset="caption" color="textSecondary" style={styles.label}>
              Password
            </Text>
            <TextInput
              placeholder="••••••••"
              value={password}
              onChangeText={setPassword}
              secureTextEntry
              autoComplete="password"
              style={styles.input}
              placeholderTextColor={theme.colors.textTertiary}
              accessibilityLabel="Password"
            />
          </View>

          <TouchableOpacity
            onPress={handleLogin}
            disabled={loading}
            style={[styles.button, loading && styles.buttonDisabled]}
            activeOpacity={0.85}
            accessibilityRole="button"
            accessibilityLabel={loading ? "Signing in" : "Sign in"}
          >
            {loading ? (
              <ActivityIndicator size="small" color={theme.colors.inverse} />
            ) : (
              <Text preset="bodyEmphasis" color="inverse">
                Sign in
              </Text>
            )}
          </TouchableOpacity>

          <TouchableOpacity
            onPress={() => Linking.openURL(`${env.adminWebUrl}/forgot-password`)}
            style={styles.link}
            accessibilityRole="link"
            accessibilityLabel="Forgot password"
          >
            <Text preset="caption" color="textSecondary">
              Forgot password?
            </Text>
          </TouchableOpacity>
        </View>

        <View style={styles.footer}>
          <Text preset="caption" color="textTertiary" align="center">
            Don&apos;t have a store yet?
          </Text>
          <TouchableOpacity
            onPress={() => Linking.openURL(env.signupUrl)}
            style={styles.signupBtn}
            activeOpacity={0.7}
            accessibilityRole="link"
            accessibilityLabel="Start a new store on mark8ly.com"
          >
            <Text preset="bodyEmphasis" color="text">
              Start a new one
            </Text>
          </TouchableOpacity>
        </View>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: theme.colors.background,
  },
  frame: {
    flex: 1,
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.huge * 1.5,
    paddingBottom: theme.spacing.xxl,
  },
  brand: {
    gap: theme.spacing.xs,
    marginBottom: theme.spacing.xxxl,
  },
  wordmark: {
    marginTop: theme.spacing.sm,
  },
  errorBox: {
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.md,
    borderLeftWidth: 2,
    borderLeftColor: theme.colors.danger,
    marginBottom: theme.spacing.lg,
    backgroundColor: theme.colors.surfaceAlt,
  },
  form: { gap: theme.spacing.lg },
  field: { gap: theme.spacing.xs },
  label: { letterSpacing: 0.4 },
  input: {
    height: 48,
    fontFamily: theme.fonts.sans,
    fontSize: 16,
    color: theme.colors.text,
    borderBottomWidth: 1,
    borderBottomColor: theme.colors.border,
    paddingHorizontal: 0,
  },
  button: {
    backgroundColor: theme.colors.text,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing.lg,
  },
  buttonDisabled: { opacity: 0.5 },
  link: {
    alignSelf: "center",
    marginTop: theme.spacing.md,
    minHeight: 44,
    justifyContent: "center",
    paddingHorizontal: theme.spacing.md,
  },
  footer: {
    marginTop: "auto",
    alignItems: "center",
    gap: theme.spacing.xs,
    paddingTop: theme.spacing.xl,
  },
  signupBtn: {
    minHeight: 44,
    justifyContent: "center",
    alignItems: "center",
    paddingHorizontal: theme.spacing.md,
  },
});

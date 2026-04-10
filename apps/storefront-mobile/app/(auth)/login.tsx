import { useState, useMemo } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  StyleSheet,
  KeyboardAvoidingView,
  Platform,
  ActivityIndicator,
  Linking,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useRouter } from "expo-router";

export default function LoginScreen() {
  const theme = useTheme();
  const { signIn } = useAuth();
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const themed = useMemo(
    () => ({
      container: { backgroundColor: theme.background },
      logo: { color: theme.text },
      subtitle: { color: theme.textSecondary },
      label: { color: theme.text },
      input: { backgroundColor: theme.elevated, color: theme.text, borderColor: theme.border },
      forgotText: { color: theme.accent },
      button: { backgroundColor: theme.primary },
      buttonText: { color: theme.elevated },
      registerLabel: { color: theme.textSecondary },
      registerLink: { color: theme.accent },
    }),
    [theme],
  );

  const handleLogin = async () => {
    const trimmedEmail = email.trim();
    if (!trimmedEmail || !password) {
      setError("Email and password are required");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      await signIn(trimmedEmail, password);
      router.back();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Login failed";
      if (
        message.includes("wrong-password") ||
        message.includes("user-not-found") ||
        message.includes("invalid-credential")
      ) {
        setError("Invalid email or password");
      } else if (message.includes("too-many-requests") || message.includes("locked")) {
        setError("Account temporarily locked — try again later");
      } else if (message.includes("network")) {
        setError("Network error — check your connection");
      } else {
        setError("Login failed — please try again");
      }
    } finally {
      setLoading(false);
    }
  };

  const handleForgotPassword = () => {
    Linking.openURL("https://mark8ly.com/reset-password");
  };

  return (
    <KeyboardAvoidingView
      style={[styles.container, themed.container]}
      behavior={Platform.OS === "ios" ? "padding" : "height"}
    >
      <View style={styles.form}>
        <Text style={[styles.logo, themed.logo]}>Mark8ly Store</Text>
        <Text style={[styles.subtitle, themed.subtitle]}>Sign in to your account</Text>

        {error && (
          <View style={styles.errorBanner}>
            <Text style={styles.errorText}>{error}</Text>
          </View>
        )}

        <View style={styles.field}>
          <Text style={[styles.label, themed.label]}>Email</Text>
          <TextInput
            placeholder="you@example.com"
            value={email}
            onChangeText={setEmail}
            autoCapitalize="none"
            keyboardType="email-address"
            autoComplete="email"
            textContentType="emailAddress"
            style={[styles.input, themed.input]}
            placeholderTextColor={theme.textSecondary}
            editable={!loading}
            accessibilityLabel="Email address"
          />
        </View>

        <View style={styles.field}>
          <Text style={[styles.label, themed.label]}>Password</Text>
          <TextInput
            placeholder="Enter your password"
            value={password}
            onChangeText={setPassword}
            secureTextEntry
            autoComplete="password"
            textContentType="password"
            style={[styles.input, themed.input]}
            placeholderTextColor={theme.textSecondary}
            editable={!loading}
            accessibilityLabel="Password"
          />
        </View>

        <Pressable
          onPress={handleForgotPassword}
          style={styles.forgotLink}
          accessibilityRole="link"
          accessibilityLabel="Forgot password"
        >
          <Text style={[styles.forgotText, themed.forgotText]}>Forgot password?</Text>
        </Pressable>

        <Pressable
          onPress={handleLogin}
          disabled={loading}
          style={[styles.button, themed.button, loading && styles.buttonDisabled]}
          accessibilityRole="button"
          accessibilityLabel="Sign in"
        >
          {loading ? (
            <ActivityIndicator size="small" color={theme.background} />
          ) : (
            <Text style={[styles.buttonText, themed.buttonText]}>Sign in</Text>
          )}
        </Pressable>

        <View style={styles.registerRow}>
          <Text style={[styles.registerLabel, themed.registerLabel]}>
            Don't have an account?
          </Text>
          <Pressable
            onPress={() => router.push("/(auth)/register")}
            accessibilityRole="link"
            accessibilityLabel="Create account"
          >
            <Text style={[styles.registerLink, themed.registerLink]}>Create account</Text>
          </Pressable>
        </View>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    paddingHorizontal: 24,
  },
  form: {
    gap: 16,
  },
  logo: {
    fontSize: 28,
    fontWeight: "700",
    fontFamily: "SourceSerif4",
    textAlign: "center",
  },
  subtitle: {
    fontSize: 16,
    textAlign: "center",
    marginBottom: 8,
  },
  errorBanner: {
    backgroundColor: "#FDE8E8",
    paddingHorizontal: 12,
    paddingVertical: 10,
    borderRadius: 6,
  },
  errorText: {
    fontSize: 13,
    color: "#8B2500",
  },
  field: {
    gap: 6,
  },
  label: {
    fontSize: 13,
    fontWeight: "600",
  },
  input: {
    borderRadius: 6,
    height: 48,
    paddingHorizontal: 14,
    fontSize: 15,
    borderWidth: 1,
  },
  forgotLink: {
    alignSelf: "flex-end",
  },
  forgotText: {
    fontSize: 13,
    fontWeight: "500",
  },
  button: {
    height: 48,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    marginTop: 4,
  },
  buttonDisabled: {
    opacity: 0.6,
  },
  buttonText: {
    fontSize: 16,
    fontWeight: "600",
  },
  registerRow: {
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    gap: 6,
    marginTop: 8,
  },
  registerLabel: {
    fontSize: 14,
  },
  registerLink: {
    fontSize: 14,
    fontWeight: "600",
  },
});

import { useState } from "react";
import { View, Text, TextInput, TouchableOpacity, StyleSheet, KeyboardAvoidingView, Platform, ActivityIndicator } from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import * as Linking from "expo-linking";
import { theme } from "@/lib/theme";

export default function LoginScreen() {
  const { signIn } = useAuth();
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
      behavior={Platform.OS === "ios" ? "padding" : "height"}
    >
      <View style={styles.form}>
        <Text style={styles.title}>Mark8ly</Text>
        <Text style={styles.subtitle}>Admin</Text>

        {error && <Text style={styles.error}>{error}</Text>}

        <TextInput
          placeholder="Email"
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          keyboardType="email-address"
          autoComplete="email"
          style={styles.input}
          placeholderTextColor={`${theme.colors.text}60`}
          accessibilityLabel="Email address"
        />

        <TextInput
          placeholder="Password"
          value={password}
          onChangeText={setPassword}
          secureTextEntry
          autoComplete="password"
          style={styles.input}
          placeholderTextColor={`${theme.colors.text}60`}
          accessibilityLabel="Password"
        />

        <TouchableOpacity
          onPress={handleLogin}
          disabled={loading}
          style={[styles.button, loading && styles.buttonDisabled]}
          activeOpacity={0.8}
          accessibilityRole="button"
          accessibilityLabel={loading ? "Signing in" : "Sign in"}
        >
          {loading ? (
            <ActivityIndicator size="small" color={theme.colors.background} />
          ) : (
            <Text style={styles.buttonText}>Sign in</Text>
          )}
        </TouchableOpacity>

        <TouchableOpacity
          onPress={() => Linking.openURL("https://admin.mark8ly.com/forgot-password")}
          style={styles.forgotLink}
          accessibilityRole="link"
          accessibilityLabel="Forgot password"
        >
          <Text style={styles.forgotText}>Forgot password?</Text>
        </TouchableOpacity>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: theme.colors.background,
    justifyContent: "center",
    paddingHorizontal: theme.spacing.xxl,
  },
  form: { gap: theme.spacing.lg },
  title: {
    fontSize: 32,
    fontWeight: "700",
    color: theme.colors.text,
  },
  subtitle: {
    fontSize: 18,
    color: theme.colors.text,
    opacity: 0.6,
    marginBottom: theme.spacing.lg,
  },
  error: { color: theme.colors.danger, fontSize: 14 },
  input: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    height: 48,
    paddingHorizontal: theme.spacing.lg,
    fontSize: 16,
    color: theme.colors.text,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}15`,
  },
  button: {
    backgroundColor: theme.colors.text,
    height: 48,
    borderRadius: theme.radius,
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing.sm,
  },
  buttonDisabled: { opacity: 0.6 },
  buttonText: { color: theme.colors.background, fontSize: 16, fontWeight: "600" },
  forgotLink: { alignItems: "center", marginTop: theme.spacing.sm },
  forgotText: { color: theme.colors.accent, fontSize: 14 },
});

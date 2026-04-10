import { useState } from "react";
import { View, Text, TextInput, TouchableOpacity, StyleSheet, KeyboardAvoidingView, Platform, ActivityIndicator } from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import * as Linking from "expo-linking";

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
          placeholderTextColor="#0E0E0C60"
        />

        <TextInput
          placeholder="Password"
          value={password}
          onChangeText={setPassword}
          secureTextEntry
          autoComplete="password"
          style={styles.input}
          placeholderTextColor="#0E0E0C60"
        />

        <TouchableOpacity
          onPress={handleLogin}
          disabled={loading}
          style={[styles.button, loading && styles.buttonDisabled]}
          activeOpacity={0.8}
        >
          {loading ? (
            <ActivityIndicator size="small" color="#F7F6F2" />
          ) : (
            <Text style={styles.buttonText}>Sign in</Text>
          )}
        </TouchableOpacity>

        <TouchableOpacity
          onPress={() => Linking.openURL("https://admin.mark8ly.com/forgot-password")}
          style={styles.forgotLink}
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
    backgroundColor: "#F7F6F2",
    justifyContent: "center",
    paddingHorizontal: 24,
  },
  form: { gap: 16 },
  title: {
    fontSize: 32,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  subtitle: {
    fontSize: 18,
    color: "#0E0E0C",
    opacity: 0.6,
    marginBottom: 16,
  },
  error: { color: "#8B2020", fontSize: 14 },
  input: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    height: 48,
    paddingHorizontal: 16,
    fontSize: 16,
    color: "#0E0E0C",
    borderWidth: 0.5,
    borderColor: "#0E0E0C15",
  },
  button: {
    backgroundColor: "#0E0E0C",
    height: 48,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    marginTop: 8,
  },
  buttonDisabled: { opacity: 0.6 },
  buttonText: { color: "#F7F6F2", fontSize: 16, fontWeight: "600" },
  forgotLink: { alignItems: "center", marginTop: 8 },
  forgotText: { color: "#2D4A2B", fontSize: 14 },
});

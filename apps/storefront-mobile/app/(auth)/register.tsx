import { useState } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  StyleSheet,
  KeyboardAvoidingView,
  Platform,
  ActivityIndicator,
  ScrollView,
} from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useRouter } from "expo-router";
import { useApiClient } from "@/lib/hooks/use-api-client";

export default function RegisterScreen() {
  const { signIn } = useAuth();
  const router = useRouter();
  const client = useApiClient();
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleRegister = async () => {
    const trimmedEmail = email.trim();
    const trimmedFirst = firstName.trim();
    const trimmedLast = lastName.trim();

    if (!trimmedFirst || !trimmedLast || !trimmedEmail || !password) {
      setError("All fields are required");
      return;
    }
    if (password.length < 8) {
      setError("Password must be at least 8 characters");
      return;
    }

    setLoading(true);
    setError(null);

    try {
      await client.post("/account/register", {
        email: trimmedEmail,
        password,
        name: `${trimmedFirst} ${trimmedLast}`,
        first_name: trimmedFirst,
        last_name: trimmedLast,
      });

      await signIn(trimmedEmail, password);
      router.back();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Registration failed";
      if (message.includes("already-exists") || message.includes("email-already-in-use")) {
        setError("An account with this email already exists");
      } else if (message.includes("weak-password")) {
        setError("Password is too weak — use at least 8 characters");
      } else if (message.includes("invalid-email")) {
        setError("Please enter a valid email address");
      } else if (message.includes("network")) {
        setError("Network error — check your connection");
      } else {
        setError("Registration failed — please try again");
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
      <ScrollView
        contentContainerStyle={styles.scrollContent}
        keyboardShouldPersistTaps="handled"
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.form}>
          <Text style={styles.logo}>Mark8ly Store</Text>
          <Text style={styles.subtitle}>Create your account</Text>

          {error && (
            <View style={styles.errorBanner}>
              <Text style={styles.errorText}>{error}</Text>
            </View>
          )}

          <View style={styles.nameRow}>
            <View style={[styles.field, styles.nameField]}>
              <Text style={styles.label}>First name</Text>
              <TextInput
                placeholder="Jane"
                value={firstName}
                onChangeText={setFirstName}
                autoCapitalize="words"
                autoComplete="given-name"
                textContentType="givenName"
                style={styles.input}
                placeholderTextColor="#999999"
                editable={!loading}
                accessibilityLabel="First name"
              />
            </View>
            <View style={[styles.field, styles.nameField]}>
              <Text style={styles.label}>Last name</Text>
              <TextInput
                placeholder="Doe"
                value={lastName}
                onChangeText={setLastName}
                autoCapitalize="words"
                autoComplete="family-name"
                textContentType="familyName"
                style={styles.input}
                placeholderTextColor="#999999"
                editable={!loading}
                accessibilityLabel="Last name"
              />
            </View>
          </View>

          <View style={styles.field}>
            <Text style={styles.label}>Email</Text>
            <TextInput
              placeholder="you@example.com"
              value={email}
              onChangeText={setEmail}
              autoCapitalize="none"
              keyboardType="email-address"
              autoComplete="email"
              textContentType="emailAddress"
              style={styles.input}
              placeholderTextColor="#999999"
              editable={!loading}
              accessibilityLabel="Email address"
            />
          </View>

          <View style={styles.field}>
            <Text style={styles.label}>Password</Text>
            <TextInput
              placeholder="At least 8 characters"
              value={password}
              onChangeText={setPassword}
              secureTextEntry
              autoComplete="new-password"
              textContentType="newPassword"
              style={styles.input}
              placeholderTextColor="#999999"
              editable={!loading}
              accessibilityLabel="Password"
            />
          </View>

          <Pressable
            onPress={handleRegister}
            disabled={loading}
            style={[styles.button, loading && styles.buttonDisabled]}
            accessibilityRole="button"
            accessibilityLabel="Create account"
          >
            {loading ? (
              <ActivityIndicator size="small" color="#F7F6F2" />
            ) : (
              <Text style={styles.buttonText}>Create account</Text>
            )}
          </Pressable>

          <View style={styles.loginRow}>
            <Text style={styles.loginLabel}>Already have an account?</Text>
            <Pressable
              onPress={() => router.back()}
              accessibilityRole="link"
              accessibilityLabel="Sign in"
            >
              <Text style={styles.loginLink}>Sign in</Text>
            </Pressable>
          </View>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#F7F6F2",
  },
  scrollContent: {
    flexGrow: 1,
    justifyContent: "center",
    paddingHorizontal: 24,
    paddingVertical: 32,
  },
  form: {
    gap: 16,
  },
  logo: {
    fontSize: 28,
    fontWeight: "700",
    color: "#0E0E0C",
    fontFamily: "SourceSerif4",
    textAlign: "center",
  },
  subtitle: {
    fontSize: 16,
    color: "#666666",
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
  nameRow: {
    flexDirection: "row",
    gap: 12,
  },
  nameField: {
    flex: 1,
  },
  field: {
    gap: 6,
  },
  label: {
    fontSize: 13,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  input: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    height: 48,
    paddingHorizontal: 14,
    fontSize: 15,
    color: "#0E0E0C",
    borderWidth: 1,
    borderColor: "#E5E4DF",
  },
  button: {
    backgroundColor: "#0E0E0C",
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
    color: "#FFFFFF",
    fontSize: 16,
    fontWeight: "600",
  },
  loginRow: {
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    gap: 6,
    marginTop: 8,
  },
  loginLabel: {
    fontSize: 14,
    color: "#666666",
  },
  loginLink: {
    fontSize: 14,
    fontWeight: "600",
    color: "#2D4A2B",
  },
});

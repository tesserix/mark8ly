import { useState, useCallback, useMemo } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  Modal,
  StyleSheet,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useAuth } from "@repo/mobile-shared/auth/provider";

interface LoginModalProps {
  visible: boolean;
  initialEmail?: string;
  onDismiss: () => void;
  onSuccess: () => void;
}

export function LoginModal({
  visible,
  initialEmail = "",
  onDismiss,
  onSuccess,
}: LoginModalProps) {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const { signIn } = useAuth();
  const [email, setEmail] = useState(initialEmail);
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSignIn = useCallback(async () => {
    if (!email.trim() || !password.trim()) {
      setError("Please enter both email and password.");
      return;
    }

    setError("");
    setLoading(true);

    try {
      await signIn(email.trim(), password);
      setPassword("");
      setError("");
      onSuccess();
    } catch (err: unknown) {
      const message =
        err instanceof Error ? err.message : "Sign in failed. Please try again.";
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [email, password, signIn, onSuccess]);

  const handleDismiss = useCallback(() => {
    setPassword("");
    setError("");
    onDismiss();
  }, [onDismiss]);

  return (
    <Modal
      visible={visible}
      transparent
      animationType="slide"
      onRequestClose={handleDismiss}
    >
      <KeyboardAvoidingView
        style={styles.overlay}
        behavior={Platform.OS === "ios" ? "padding" : "height"}
      >
        <View style={styles.card}>
          <Text style={styles.title}>Sign in</Text>
          <Text style={styles.subtitle}>
            Sign in to access saved addresses and order history.
          </Text>

          {error.length > 0 && (
            <View style={styles.errorBanner}>
              <Text style={styles.errorText}>{error}</Text>
            </View>
          )}

          <View style={styles.field}>
            <Text style={styles.label}>Email</Text>
            <TextInput
              style={styles.input}
              value={email}
              onChangeText={setEmail}
              placeholder="you@example.com"
              placeholderTextColor={theme.textSecondary}
              keyboardType="email-address"
              autoCapitalize="none"
              autoComplete="email"
              textContentType="emailAddress"
              editable={!loading}
              accessibilityLabel="Email address"
            />
          </View>

          <View style={styles.field}>
            <Text style={styles.label}>Password</Text>
            <TextInput
              style={styles.input}
              value={password}
              onChangeText={setPassword}
              placeholder="Enter your password"
              placeholderTextColor={theme.textSecondary}
              secureTextEntry
              autoComplete="password"
              textContentType="password"
              editable={!loading}
              accessibilityLabel="Password"
            />
          </View>

          <Pressable
            style={[styles.signInButton, loading && styles.signInButtonLoading]}
            onPress={handleSignIn}
            disabled={loading}
            accessibilityRole="button"
            accessibilityLabel="Sign in"
          >
            {loading ? (
              <ActivityIndicator size="small" color={theme.elevated} />
            ) : (
              <Text style={styles.signInButtonText}>Sign in</Text>
            )}
          </Pressable>

          <Pressable
            style={styles.cancelButton}
            onPress={handleDismiss}
            disabled={loading}
            accessibilityRole="button"
            accessibilityLabel="Cancel sign in"
          >
            <Text style={styles.cancelText}>Cancel</Text>
          </Pressable>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  overlay: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.4)",
    justifyContent: "flex-end",
  },
  card: {
    backgroundColor: theme.elevated,
    borderTopLeftRadius: 16,
    borderTopRightRadius: 16,
    padding: 24,
    paddingBottom: 40,
    gap: 16,
  },
  title: {
    fontSize: 20,
    fontWeight: "700",
    color: theme.text,
    fontFamily: "SourceSerif4",
  },
  subtitle: {
    fontSize: 14,
    color: theme.textSecondary,
    lineHeight: 20,
  },
  errorBanner: {
    backgroundColor: "#FDE8E8",
    paddingHorizontal: 12,
    paddingVertical: 8,
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
    color: theme.text,
  },
  input: {
    borderWidth: 1,
    borderColor: theme.border,
    borderRadius: 6,
    paddingHorizontal: 12,
    paddingVertical: 12,
    fontSize: 15,
    color: theme.text,
    backgroundColor: theme.elevated,
  },
  signInButton: {
    backgroundColor: theme.primary,
    paddingVertical: 14,
    borderRadius: 6,
    alignItems: "center",
    marginTop: 4,
  },
  signInButtonLoading: {
    opacity: 0.7,
  },
  signInButtonText: {
    color: theme.elevated,
    fontSize: 15,
    fontWeight: "600",
  },
  cancelButton: {
    alignItems: "center",
    paddingVertical: 8,
  },
  cancelText: {
    fontSize: 14,
    color: theme.textSecondary,
  },
});
}

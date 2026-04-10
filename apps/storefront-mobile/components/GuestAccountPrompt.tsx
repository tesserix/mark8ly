import { useState, useCallback, useMemo } from "react";
import {
  View,
  Text,
  TextInput,
  Pressable,
  StyleSheet,
  ActivityIndicator,
  Alert,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useCreateGuestAccount } from "@/lib/hooks/use-checkout";

interface GuestAccountPromptProps {
  email: string;
  name: string;
  onDismiss: () => void;
}

export function GuestAccountPrompt({
  email,
  name,
  onDismiss,
}: GuestAccountPromptProps) {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const [password, setPassword] = useState("");
  const [dismissed, setDismissed] = useState(false);
  const createAccount = useCreateGuestAccount();

  const handleCreate = useCallback(async () => {
    if (password.length < 8) {
      Alert.alert("Password too short", "Please use at least 8 characters.");
      return;
    }

    try {
      await createAccount.mutateAsync({ email, password, name });
      Alert.alert("Account created", "You can now sign in for faster checkout.");
      setDismissed(true);
    } catch {
      Alert.alert("Error", "Could not create account. Please try again.");
    }
  }, [email, name, password, createAccount]);

  const handleDismiss = useCallback(() => {
    setDismissed(true);
    onDismiss();
  }, [onDismiss]);

  if (dismissed) return null;

  return (
    <View style={styles.container}>
      <Text style={styles.title}>Save your details?</Text>
      <Text style={styles.subtitle}>
        Create an account for faster checkout next time.
      </Text>

      <View style={styles.field}>
        <Text style={styles.label}>Email</Text>
        <TextInput
          style={[styles.input, styles.inputDisabled]}
          value={email}
          editable={false}
          accessibilityLabel="Email address"
        />
      </View>

      <View style={styles.field}>
        <Text style={styles.label}>Password</Text>
        <TextInput
          style={styles.input}
          value={password}
          onChangeText={setPassword}
          placeholder="At least 8 characters"
          placeholderTextColor={theme.textSecondary}
          secureTextEntry
          autoComplete="new-password"
          textContentType="newPassword"
          accessibilityLabel="Password"
        />
      </View>

      <Pressable
        style={[
          styles.createButton,
          createAccount.isPending && styles.createButtonLoading,
        ]}
        onPress={handleCreate}
        disabled={createAccount.isPending}
        accessibilityRole="button"
        accessibilityLabel="Create account"
      >
        {createAccount.isPending ? (
          <ActivityIndicator size="small" color={theme.elevated} />
        ) : (
          <Text style={styles.createButtonText}>Create account</Text>
        )}
      </Pressable>

      <Pressable
        style={styles.dismissButton}
        onPress={handleDismiss}
        accessibilityRole="button"
        accessibilityLabel="No thanks"
      >
        <Text style={styles.dismissText}>No thanks</Text>
      </Pressable>
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    backgroundColor: theme.elevated,
    borderRadius: 6,
    padding: 16,
    gap: 12,
    borderWidth: 1,
    borderColor: theme.border,
  },
  title: {
    fontSize: 16,
    fontWeight: "700",
    color: theme.text,
    fontFamily: "SourceSerif4",
  },
  subtitle: {
    fontSize: 14,
    color: theme.textSecondary,
    lineHeight: 20,
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
    paddingVertical: 10,
    fontSize: 14,
    color: theme.text,
    backgroundColor: theme.elevated,
  },
  inputDisabled: {
    backgroundColor: theme.background,
    color: theme.textSecondary,
  },
  createButton: {
    backgroundColor: theme.primary,
    paddingVertical: 14,
    borderRadius: 6,
    alignItems: "center",
  },
  createButtonLoading: {
    opacity: 0.7,
  },
  createButtonText: {
    color: theme.elevated,
    fontSize: 15,
    fontWeight: "600",
  },
  dismissButton: {
    alignItems: "center",
    paddingVertical: 4,
  },
  dismissText: {
    fontSize: 14,
    color: theme.textSecondary,
  },
});
}

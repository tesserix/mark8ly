import { useCallback, useState } from "react";
import { View, ScrollView, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { ApiError } from "@repo/mobile-shared/api/client";
import { ASSIGNABLE_ROLES } from "@repo/mobile-shared/api/schemas/team";
import { useInviteTeamMember } from "@/lib/admin-api/team-actions";
import { BackHeader, FieldInput, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

const ROLE_HINT: Record<string, string> = {
  admin: "Manage everything except billing and ownership",
  staff: "Manage orders, products and customers",
  viewer: "Read-only access",
};

function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function looksLikeEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());
}

export default function InviteTeammateScreen() {
  const router = useRouter();
  const invite = useInviteTeamMember();

  const [email, setEmail] = useState("");
  const [role, setRole] = useState<string>("staff");

  const canSubmit = looksLikeEmail(email) && !invite.isPending;

  const onSubmit = useCallback(() => {
    if (!canSubmit) return;
    invite.mutate(
      { email: email.trim(), role },
      {
        onSuccess: () => router.back(),
        onError: (err) =>
          Alert.alert(
            "Couldn't send invite",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      },
    );
  }, [canSubmit, invite, email, role, router]);

  return (
    <Screen>
      <BackHeader eyebrow="TEAM" title="Invite teammate" />
      <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
        <FieldInput
          label="Email"
          value={email}
          onChangeText={setEmail}
          placeholder="teammate@example.com"
          keyboardType="email-address"
          autoCapitalize="none"
          autoFocus
        />

        <Text preset="caption" color="textTertiary">
          Role
        </Text>
        <View style={styles.roles}>
          {ASSIGNABLE_ROLES.map((r) => {
            const selected = r === role;
            return (
              <Pressable
                key={r}
                style={[styles.roleCard, selected && styles.roleCardSelected]}
                onPress={() => setRole(r)}
                accessibilityRole="button"
                accessibilityState={{ selected }}
                accessibilityLabel={`${titleize(r)}. ${ROLE_HINT[r]}`}
              >
                <Text preset="bodyEmphasis" color={selected ? "inverse" : "text"}>
                  {titleize(r)}
                </Text>
                <Text preset="caption" color={selected ? "inverse" : "textTertiary"}>
                  {ROLE_HINT[r]}
                </Text>
              </Pressable>
            );
          })}
        </View>

        <Pressable
          style={[styles.btn, !canSubmit && styles.disabled]}
          onPress={onSubmit}
          disabled={!canSubmit}
          accessibilityRole="button"
          accessibilityLabel="Send invite"
        >
          {invite.isPending ? (
            <ActivityIndicator size="small" color={theme.colors.inverse} />
          ) : (
            <Text preset="bodyEmphasis" color="inverse">
              Send invite
            </Text>
          )}
        </Pressable>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: theme.spacing.lg, gap: theme.spacing.md, paddingBottom: theme.spacing.huge },
  roles: { gap: theme.spacing.sm },
  roleCard: {
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    padding: theme.spacing.md,
    gap: 2,
  },
  roleCardSelected: { backgroundColor: theme.colors.text, borderColor: theme.colors.text },
  btn: {
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing.sm,
  },
  disabled: { opacity: 0.4 },
});

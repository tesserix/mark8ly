import { useCallback } from "react";
import { View, ScrollView, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { UserPlus, X } from "lucide-react-native";
import { ApiError } from "@repo/mobile-shared/api/client";
import { ASSIGNABLE_ROLES } from "@repo/mobile-shared/api/schemas/team";
import { useTeamMembers, useTeamInvitations } from "@/lib/hooks/use-team";
import { useUpdateMemberRole, useRevokeInvitation } from "@/lib/admin-api/team-actions";
import { BackHeader, Eyebrow, EmptyState, Hairline, IconButton, PressableRow, Screen, StatusBadge, Text, type StatusTone } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import { Alert } from "react-native";
import type { TeamMember, TeamInvitation } from "@repo/mobile-shared/api/types";

const ROLE_TONE: Record<string, StatusTone> = {
  owner: "neutral",
  admin: "info",
  staff: "muted",
  viewer: "muted",
};

function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function RevokeInviteButton({
  disabled,
  accessibilityLabel,
  onPress,
}: {
  disabled: boolean;
  accessibilityLabel: string;
  onPress: () => void;
}) {
  return (
    <IconButton onPress={onPress} disabled={disabled} accessibilityLabel={accessibilityLabel} tone="danger">
      <X size={18} color={theme.colors.danger} strokeWidth={2} />
    </IconButton>
  );
}

export default function TeamScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const members = useTeamMembers();
  const invitations = useTeamInvitations();
  const updateRole = useUpdateMemberRole();
  const revoke = useRevokeInvitation();

  const refreshing = members.isRefetching || invitations.isRefetching;
  const onRefresh = useCallback(() => {
    members.refetch();
    invitations.refetch();
  }, [members, invitations]);

  const onChangeRole = useCallback(
    (member: TeamMember) => {
      if (member.kind === "owner") return;
      Alert.alert(
        "Change role",
        member.email,
        [
          ...ASSIGNABLE_ROLES.map((r) => ({
            text: titleize(r),
            onPress: () =>
              updateRole.mutate(
                { email: member.email, newRole: r },
                {
                  onError: (err) =>
                    Alert.alert(
                      "Couldn't change role",
                      err instanceof ApiError ? err.message : "Please try again.",
                    ),
                },
              ),
          })),
          { text: "Cancel", style: "cancel" as const },
        ],
      );
    },
    [updateRole],
  );

  const onRevoke = useCallback(
    (inv: TeamInvitation) => {
      Alert.alert("Revoke invitation", `Cancel the invite to ${inv.email}?`, [
        { text: "Keep", style: "cancel" },
        {
          text: "Revoke",
          style: "destructive",
          onPress: () =>
            revoke.mutate(inv.id, {
              onError: (err) =>
                Alert.alert(
                  "Couldn't revoke",
                  err instanceof ApiError ? err.message : "Please try again.",
                ),
            }),
        },
      ]);
    },
    [revoke],
  );

  const memberList = members.data?.data ?? [];
  const inviteList = invitations.data?.data ?? [];
  const loading = members.isLoading || invitations.isLoading;

  return (
    <Screen>
      <BackHeader
        eyebrow="SETTINGS"
        title="Team"
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/settings/team/invite")}
            accessibilityLabel="Invite teammate"
          >
            <UserPlus size={20} color={theme.colors.text} strokeWidth={1.75} />
          </IconButton>
        }
      />
      {loading && !refreshing ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : (members.isError || invitations.isError) && memberList.length === 0 && inviteList.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load team"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { onRefresh(); } }}
          />
        </View>
      ) : (
        <ScrollView
          contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}
          refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.text} />}
        >
          <Eyebrow label="Members" style={styles.section} />
          <View style={styles.card}>
            {memberList.map((m, i) => {
              // The owner row — and every row while a role change is in
              // flight — is non-interactive: no tap does anything. Rendered
              // via PressableRow's `disabled` prop rather than a separate
              // plain View, so it still gets the row's layout/background for
              // free and suppresses press feedback and the "Tap to change
              // role" affordance announced to VoiceOver automatically.
              const disabled = m.kind === "owner" || updateRole.isPending;
              const rowContent = (
                <>
                  <Text preset="body" color="text" numberOfLines={1} style={styles.email}>
                    {m.email}
                  </Text>
                  <StatusBadge label={titleize(m.role)} tone={ROLE_TONE[m.role] ?? "muted"} />
                </>
              );
              return (
                <View key={m.email}>
                  {i > 0 ? <Hairline /> : null}
                  <PressableRow
                    style={styles.memberRow}
                    onPress={() => onChangeRole(m)}
                    disabled={disabled}
                    accessibilityLabel={
                      disabled
                        ? `${m.email}, ${m.role}`
                        : `${m.email}, ${m.role}. Tap to change role`
                    }
                  >
                    {rowContent}
                  </PressableRow>
                </View>
              );
            })}
          </View>

          <Eyebrow label="Pending invitations" style={styles.section} />
          <View style={styles.card}>
            {inviteList.length === 0 ? (
              <Text preset="body" color="textTertiary">
                No pending invitations.
              </Text>
            ) : (
              inviteList.map((inv, i) => (
                <View key={inv.id}>
                  {i > 0 ? <Hairline /> : null}
                  <View style={styles.row}>
                    <View style={styles.inviteInfo}>
                      <Text preset="body" color="text" numberOfLines={1}>
                        {inv.email}
                      </Text>
                      <Text preset="caption" color="textTertiary">
                        {titleize(inv.role)} · {titleize(inv.status)}
                      </Text>
                    </View>
                    <RevokeInviteButton
                      onPress={() => onRevoke(inv)}
                      disabled={revoke.isPending}
                      accessibilityLabel={`Revoke invite to ${inv.email}`}
                    />
                  </View>
                </View>
              ))
            )}
          </View>

          {memberList.length === 0 && inviteList.length === 0 ? (
            <EmptyState align="left" title="No team yet" message="Invite teammates to help run your store." />
          ) : null}

          <Text preset="caption" color="textTertiary" style={styles.note}>
            The owner role is fixed. Tap a member to change their role.
          </Text>
        </ScrollView>
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so
  // this screen's rows and section labels sit flush with every other list
  // screen. Not theme.spacing.lg — that token is shared with non-gutter
  // spacing throughout the app and must not move.
  section: { paddingHorizontal: theme.spacing.xl, paddingTop: theme.spacing.xl },
  card: { paddingHorizontal: theme.spacing.xl, marginTop: theme.spacing.sm },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: theme.spacing.md,
    gap: theme.spacing.md,
  },
  // PressableRow already owns flexDirection/alignItems/padding/gap. Two
  // overrides: `justifyContent` for the email/role-badge space-between, and
  // `paddingHorizontal: 0` — the wrapping `card` above already adds the
  // screen gutter, and PressableRow's own base adds theme.row.paddingH (20)
  // on top of that, which would indent member rows a further 20pt past the
  // plain-View invitation rows below (also inside `card`, no row padding of
  // their own) — visibly misaligned under the same eyebrows. Zeroing it here
  // keeps both columns flush at the card's gutter. No backgroundColor
  // override — this list sits directly on Screen's paper background (no
  // Card/elevated wrapper here), which is exactly PressableRow's own
  // default, so the pre-migration transparency was already correct.
  // Owner/pending-mutation rows use the same style via PressableRow's
  // `disabled` prop instead of a hand-mirrored static twin — see
  // PressableRow.tsx.
  memberRow: { justifyContent: "space-between", paddingHorizontal: 0 },
  email: { flexShrink: 1 },
  inviteInfo: { flex: 1, gap: 2 },
  note: { paddingHorizontal: theme.spacing.xl, paddingTop: theme.spacing.md },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  // NOT `styles.centered` for the error state: that wrapper's
  // `alignItems: "center"` shrink-wraps and re-centres its child, silently
  // undoing `EmptyState align="left"`. Claims the remaining height only.
  errorSlot: { flex: 1 },
});

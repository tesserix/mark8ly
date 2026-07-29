import { useCallback } from "react";
import { View, ScrollView, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { UserPlus, X } from "lucide-react-native";
import { ApiError } from "@repo/mobile-shared/api/client";
import { ASSIGNABLE_ROLES } from "@repo/mobile-shared/api/schemas/team";
import { useTeamMembers, useTeamInvitations } from "@/lib/hooks/use-team";
import { useUpdateMemberRole, useRevokeInvitation } from "@/lib/admin-api/team-actions";
import {
  BackHeader,
  EmptyState,
  GroupedList,
  GroupedRow,
  IconButton,
  Screen,
  StatusBadge,
  Text,
  type StatusTone,
} from "@/components/ui";
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
          <GroupedList
            sections={[
              {
                key: "members",
                label: "Members",
                rows: memberList.map((m) => {
                  // The owner row — and every row while a role change is in
                  // flight — is non-interactive: no tap does anything.
                  // Omitting `onPress` now produces a genuinely
                  // non-interactive row (GroupedRow's plain-View branch)
                  // rather than a disabled PressableRow, which announced as
                  // a dimmed button to VoiceOver even though nothing was
                  // pressable.
                  const disabled = m.kind === "owner" || updateRole.isPending;
                  return (
                    <GroupedRow
                      key={m.email}
                      label={m.email}
                      trailing={<StatusBadge label={titleize(m.role)} tone={ROLE_TONE[m.role] ?? "muted"} />}
                      onPress={disabled ? undefined : () => onChangeRole(m)}
                      accessibilityLabel={
                        disabled
                          ? `${m.email}, ${m.role}`
                          : `${m.email}, ${m.role}. Tap to change role`
                      }
                    />
                  );
                }),
              },
            ]}
          />

          {inviteList.length === 0 ? (
            <View style={styles.emptySection}>
              <Text preset="eyebrow" color="textTertiary" style={styles.emptySectionLabel}>
                Pending invitations
              </Text>
              <Text preset="body" color="textTertiary" style={styles.emptySectionLabel}>
                No pending invitations.
              </Text>
            </View>
          ) : (
            <GroupedList
              sections={[
                {
                  key: "invitations",
                  label: "Pending invitations",
                  // Invitations are always non-interactive — no `onPress` —
                  // the only press target on the row is Revoke, in trailing.
                  rows: inviteList.map((inv) => (
                    <GroupedRow
                      key={inv.id}
                      label={inv.email}
                      hint={`${titleize(inv.role)} · ${titleize(inv.status)}`}
                      trailing={
                        <RevokeInviteButton
                          onPress={() => onRevoke(inv)}
                          disabled={revoke.isPending}
                          accessibilityLabel={`Revoke invite to ${inv.email}`}
                        />
                      }
                    />
                  )),
                },
              ]}
            />
          )}

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
  // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so
  // GroupedList's eyebrows/cards and the two hand-rolled blocks below (the
  // empty-invitations copy, the closing note) share one left edge. Not
  // theme.spacing.lg — that token is shared with non-gutter spacing
  // throughout the app and must not move.
  scroll: {
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xs,
    gap: theme.spacing.lg,
  },
  // Mirrors GroupedList's own eyebrow + row styling exactly, for the one
  // state GroupedList itself doesn't render: zero invitations. An empty
  // Card would be a visible blank box; this keeps the pre-migration "No
  // pending invitations." copy instead.
  emptySection: { gap: theme.spacing.sm },
  emptySectionLabel: { paddingHorizontal: theme.spacing.xs },
  note: { paddingTop: theme.spacing.md },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  // NOT `styles.centered` for the error state: that wrapper's
  // `alignItems: "center"` shrink-wraps and re-centres its child, silently
  // undoing `EmptyState align="left"`. Claims the remaining height only.
  errorSlot: { flex: 1 },
});

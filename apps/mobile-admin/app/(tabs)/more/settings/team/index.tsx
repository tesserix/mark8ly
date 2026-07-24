import { useCallback } from "react";
import { View, ScrollView, TouchableOpacity, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { UserPlus, X } from "lucide-react-native";
import { ApiError } from "@repo/mobile-shared/api/client";
import { ASSIGNABLE_ROLES } from "@repo/mobile-shared/api/schemas/team";
import { useTeamMembers, useTeamInvitations } from "@/lib/hooks/use-team";
import { useUpdateMemberRole, useRevokeInvitation } from "@/lib/admin-api/team-actions";
import { BackHeader, Eyebrow, EmptyState, Hairline, Screen, StatusBadge, Text, type StatusTone } from "@/components/ui";
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
          <TouchableOpacity
            onPress={() => router.push("/(tabs)/more/settings/team/invite")}
            hitSlop={12}
            accessibilityRole="button"
            accessibilityLabel="Invite teammate"
          >
            <UserPlus size={20} color={theme.colors.text} strokeWidth={1.75} />
          </TouchableOpacity>
        }
      />
      {loading && !refreshing ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : (members.isError || invitations.isError) && memberList.length === 0 && inviteList.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
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
            {memberList.map((m, i) => (
              <View key={m.email}>
                {i > 0 ? <Hairline /> : null}
                <TouchableOpacity
                  style={styles.row}
                  onPress={() => onChangeRole(m)}
                  disabled={m.kind === "owner" || updateRole.isPending}
                  activeOpacity={0.6}
                  accessibilityRole="button"
                  accessibilityLabel={`${m.email}, ${m.role}${m.kind === "owner" ? "" : ". Tap to change role"}`}
                >
                  <Text preset="body" color="text" numberOfLines={1} style={styles.email}>
                    {m.email}
                  </Text>
                  <StatusBadge label={titleize(m.role)} tone={ROLE_TONE[m.role] ?? "muted"} />
                </TouchableOpacity>
              </View>
            ))}
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
                    <TouchableOpacity
                      onPress={() => onRevoke(inv)}
                      disabled={revoke.isPending}
                      hitSlop={10}
                      accessibilityRole="button"
                      accessibilityLabel={`Revoke invite to ${inv.email}`}
                    >
                      <X size={18} color={theme.colors.danger} strokeWidth={2} />
                    </TouchableOpacity>
                  </View>
                </View>
              ))
            )}
          </View>

          {memberList.length === 0 && inviteList.length === 0 ? (
            <EmptyState title="No team yet" message="Invite teammates to help run your store." />
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
  section: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl },
  card: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: theme.spacing.md,
    gap: theme.spacing.md,
  },
  email: { flexShrink: 1 },
  inviteInfo: { flex: 1, gap: 2 },
  note: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.md },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
});

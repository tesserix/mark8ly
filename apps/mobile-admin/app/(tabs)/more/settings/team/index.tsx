import { useCallback, useState } from "react";
import { Platform, View, ScrollView, Pressable, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { UserPlus, X } from "lucide-react-native";
import { ApiError } from "@repo/mobile-shared/api/client";
import { ASSIGNABLE_ROLES } from "@repo/mobile-shared/api/schemas/team";
import { useTeamMembers, useTeamInvitations } from "@/lib/hooks/use-team";
import { useUpdateMemberRole, useRevokeInvitation } from "@/lib/admin-api/team-actions";
import { BackHeader, Eyebrow, EmptyState, Hairline, PressableRow, Screen, StatusBadge, Text, type StatusTone } from "@/components/ui";
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

// Extracted so press state can live in `useState`: this button renders once
// per row inside `inviteList.map()`, and hooks can't be called from inside a
// `.map()` callback — each row needs its own component instance instead.
function RevokeInviteButton({
  disabled,
  accessibilityLabel,
  onPress,
}: {
  disabled: boolean;
  accessibilityLabel: string;
  onPress: () => void;
}) {
  const [pressed, setPressed] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onPressIn={() => setPressed(true)}
      onPressOut={() => setPressed(false)}
      disabled={disabled}
      hitSlop={10}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      android_ripple={{ ...theme.press.rippleDanger, borderless: true }}
      style={[
        pressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
      ]}
    >
      <X size={18} color={theme.colors.danger} strokeWidth={2} />
    </Pressable>
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
  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [invitePressed, setInvitePressed] = useState(false);

  return (
    <Screen>
      <BackHeader
        eyebrow="SETTINGS"
        title="Team"
        rightSlot={
          <Pressable
            onPress={() => router.push("/(tabs)/more/settings/team/invite")}
            onPressIn={() => setInvitePressed(true)}
            onPressOut={() => setInvitePressed(false)}
            hitSlop={12}
            accessibilityRole="button"
            accessibilityLabel="Invite teammate"
            android_ripple={{ ...theme.press.rippleInk, borderless: true }}
            style={[
              invitePressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
            ]}
          >
            <UserPlus size={20} color={theme.colors.text} strokeWidth={1.75} />
          </Pressable>
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
            {memberList.map((m, i) => {
              // PressableRow has no `disabled` prop (its public API is
              // fixed) and would otherwise show full sink/ripple feedback
              // for a tap that does nothing. Render the owner row — and
              // every row while a role change is in flight — as a plain,
              // non-interactive View instead, same precedent as
              // app/notifications.tsx's NotificationItem: no
              // accessibilityRole="button", no press feedback, and no false
              // "Tap to change role" affordance announced to VoiceOver.
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
                  {disabled ? (
                    <View
                      style={styles.memberRowStatic}
                      accessible={true}
                      accessibilityLabel={`${m.email}, ${m.role}`}
                    >
                      {rowContent}
                    </View>
                  ) : (
                    <PressableRow
                      style={styles.memberRow}
                      onPress={() => onChangeRole(m)}
                      accessibilityLabel={`${m.email}, ${m.role}. Tap to change role`}
                    >
                      {rowContent}
                    </PressableRow>
                  )}
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
  // PressableRow already owns flexDirection/alignItems/padding/gap. Two
  // overrides: `justifyContent` for the email/role-badge space-between, and
  // `paddingHorizontal: 0` — the wrapping `card` above already adds
  // theme.spacing.lg (16), and PressableRow's own base adds theme.row.paddingH
  // (20) on top of that, which indented member rows a further 20pt past the
  // plain-View invitation rows below (also inside `card`, no row padding of
  // their own) — 36pt vs 16pt, visibly misaligned under the same eyebrows.
  // Zeroing it here keeps both columns flush at the card's 16pt. No
  // backgroundColor override — this list sits directly on Screen's paper
  // background (no Card/elevated wrapper here), which is exactly
  // PressableRow's own default, so the pre-migration transparency was
  // already correct.
  memberRow: { justifyContent: "space-between", paddingHorizontal: 0 },
  // Non-interactive twin of `memberRow` for the owner/pending-mutation
  // case — PressableRow can't be reused here (no `disabled` prop), so this
  // manually mirrors its base layout (flexDirection/alignItems/gap/padding/
  // minHeight/background) rather than PressableRow's own token, so a future
  // change to PressableRow's density doesn't silently drift these apart.
  memberRowStatic: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: theme.row.gap,
    paddingVertical: theme.row.paddingV,
    minHeight: theme.row.minHeightSingle,
    backgroundColor: theme.colors.background,
  },
  email: { flexShrink: 1 },
  inviteInfo: { flex: 1, gap: 2 },
  note: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.md },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
});

import { useCallback } from "react";
import {
  View,
  FlatList,
  TouchableOpacity,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useNotifications, useMarkAllRead } from "../../../lib/hooks/use-notifications";
import {
  BackHeader,
  EmptyState,
  Hairline,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Notification } from "@repo/mobile-shared/api/types";

// Real values from notification/models.go:16-30. The previous map used
// order/payment/alert/system, which match NOTHING the backend emits, so every
// notification silently fell through to the default colour.
const TYPE_DOT: Record<string, string> = {
  new_order: theme.colors.accent,
  order_fulfilled: theme.colors.accent,
  order_cancelled: theme.colors.danger,
  payment_received: theme.colors.warning,
  low_stock: theme.colors.warning,
  return_requested: theme.colors.warning,
  review_submitted: theme.colors.text,
  system_alert: theme.colors.danger,
};

function formatRelativeTime(dateString: string): string {
  const now = Date.now();
  const date = new Date(dateString).getTime();
  const diffMs = now - date;
  const diffMin = Math.floor(diffMs / 60_000);
  const diffHr = Math.floor(diffMs / 3_600_000);
  const diffDay = Math.floor(diffMs / 86_400_000);
  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHr < 24) return `${diffHr}h ago`;
  if (diffDay < 30) return `${diffDay}d ago`;
  return new Date(dateString).toLocaleDateString();
}

function NotificationItem({
  notification,
  onPress,
}: {
  notification: Notification;
  onPress: (n: Notification) => void;
}) {
  const dotColor = TYPE_DOT[notification.type] ?? theme.colors.text;
  const isUnread = !notification.is_read;
  const message = notification.message ?? "";

  return (
    <TouchableOpacity
      style={[styles.row, isUnread && styles.rowUnread]}
      onPress={() => onPress(notification)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`${notification.title}, ${message}, ${isUnread ? "unread" : "read"}`}
    >
      {isUnread ? <View style={[styles.unreadBar, { backgroundColor: dotColor }]} /> : null}
      <View style={[styles.dot, { backgroundColor: dotColor }]} />
      <View style={styles.content}>
        <Text
          preset="bodyEmphasis"
          color="text"
          numberOfLines={1}
          style={isUnread ? styles.unreadTitle : undefined}
        >
          {notification.title}
        </Text>
        <Text preset="caption" color="textSecondary" numberOfLines={2} style={styles.body}>
          {message}
        </Text>
        <Text preset="caption" color="textTertiary">
          {formatRelativeTime(notification.created_at)}
        </Text>
      </View>
    </TouchableOpacity>
  );
}

export default function NotificationsScreen() {
  const { data, isLoading, isRefetching, refetch } = useNotifications();
  const markAllRead = useMarkAllRead();

  const hasUnread = data?.notifications.some((n) => !n.is_read) ?? false;

  // The wire has no deep_link — it sends resource_type/resource_id instead.
  // Mapping those to routes would be pure guesswork: the endpoint is empty in
  // prod, so no real notification has ever been observed. Deferred until there
  // is data to verify against.
  const handlePress = useCallback((_notification: Notification) => {}, []);

  return (
    <Screen>
      <BackHeader
        eyebrow="NOTIFICATIONS"
        title="Inbox"
        rightSlot={
          hasUnread ? (
            <TouchableOpacity
              onPress={() => markAllRead.mutate()}
              disabled={markAllRead.isPending}
              hitSlop={8}
              accessibilityRole="button"
              accessibilityLabel={
                markAllRead.isPending ? "Marking all as read" : "Mark all as read"
              }
            >
              <Text preset="caption" color="accent">
                {markAllRead.isPending ? "Marking…" : "Mark all"}
              </Text>
            </TouchableOpacity>
          ) : null
        }
      />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : (
        <FlatList
          data={data?.notifications ?? []}
          renderItem={({ item }) => <NotificationItem notification={item} onPress={handlePress} />}
          keyExtractor={(item) => item.id}
          ItemSeparatorComponent={() => <Hairline />}
          contentContainerStyle={styles.list}
          refreshControl={
            <RefreshControl
              refreshing={isRefetching}
              onRefresh={refetch}
              tintColor={theme.colors.text}
            />
          }
          ListEmptyComponent={
            <EmptyState title="No notifications" message="You're all caught up." />
          }
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  list: { flexGrow: 1, paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  row: {
    flexDirection: "row",
    backgroundColor: theme.colors.elevated,
    paddingVertical: theme.spacing.md,
    paddingHorizontal: theme.spacing.lg,
    gap: theme.spacing.md,
  },
  rowUnread: { backgroundColor: theme.colors.surfaceAlt },
  unreadBar: {
    position: "absolute",
    left: 0,
    top: 0,
    bottom: 0,
    width: 3,
  },
  dot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    marginTop: 6,
  },
  content: { flex: 1, gap: 2 },
  unreadTitle: { fontWeight: "700" },
  body: { marginVertical: 2 },
});

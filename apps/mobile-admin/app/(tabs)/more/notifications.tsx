import { useCallback } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { useNotifications, useMarkAllRead } from "../../../lib/hooks/use-notifications";
import { theme } from "@/lib/theme";
import type { Notification } from "@repo/mobile-shared/api/types";

const TYPE_COLORS: Record<string, string> = {
  order: theme.colors.accent,
  payment: "#F59E0B",
  alert: theme.colors.danger,
  system: theme.colors.text,
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
  const dotColor = TYPE_COLORS[notification.type] ?? theme.colors.text;
  const isUnread = !notification.read;

  return (
    <TouchableOpacity
      style={[styles.notifRow, isUnread && styles.notifRowUnread]}
      onPress={() => onPress(notification)}
      activeOpacity={0.7}
      accessibilityRole="button"
      accessibilityLabel={`${notification.title}, ${notification.body}, ${isUnread ? "unread" : "read"}`}
    >
      {isUnread && <View style={[styles.unreadBorder, { backgroundColor: dotColor }]} />}
      <View style={styles.dotContainer}>
        <View style={[styles.dot, { backgroundColor: dotColor }]} />
      </View>
      <View style={styles.notifContent}>
        <Text
          style={[styles.notifTitle, isUnread && styles.notifTitleUnread]}
          numberOfLines={1}
        >
          {notification.title}
        </Text>
        <Text style={styles.notifBody} numberOfLines={2}>
          {notification.body}
        </Text>
        <Text style={styles.notifTime}>
          {formatRelativeTime(notification.created_at)}
        </Text>
      </View>
    </TouchableOpacity>
  );
}

export default function NotificationsScreen() {
  const router = useRouter();
  const { data, isLoading, isRefetching, refetch } = useNotifications();
  const markAllRead = useMarkAllRead();

  const hasUnread = data?.items.some((n) => !n.read) ?? false;

  const handlePress = useCallback(
    (notification: Notification) => {
      if (notification.deep_link) {
        router.push(notification.deep_link as never);
      }
    },
    [router],
  );

  const renderItem = useCallback(
    ({ item }: { item: Notification }) => (
      <NotificationItem notification={item} onPress={handlePress} />
    ),
    [handlePress],
  );

  const keyExtractor = useCallback((item: Notification) => item.id, []);

  return (
    <View style={styles.screen}>
      {hasUnread && (
        <TouchableOpacity
          style={styles.markAllBtn}
          onPress={() => markAllRead.mutate()}
          disabled={markAllRead.isPending}
          accessibilityRole="button"
          accessibilityLabel={markAllRead.isPending ? "Marking all as read" : "Mark all as read"}
        >
          <Text style={styles.markAllText}>
            {markAllRead.isPending ? "Marking..." : "Mark all as read"}
          </Text>
        </TouchableOpacity>
      )}

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="large" color={theme.colors.text} />
        </View>
      ) : (
        <FlatList
          data={data?.items ?? []}
          renderItem={renderItem}
          keyExtractor={keyExtractor}
          contentContainerStyle={styles.listContent}
          refreshControl={
            <RefreshControl
              refreshing={isRefetching}
              onRefresh={refetch}
              tintColor={theme.colors.text}
            />
          }
          ListEmptyComponent={
            <View style={styles.centered}>
              <Text style={styles.emptyTitle}>No notifications</Text>
              <Text style={styles.emptySubtitle}>
                You're all caught up
              </Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: theme.colors.background,
  },
  markAllBtn: {
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: 10,
    alignItems: "flex-end",
    minHeight: 44,
    justifyContent: "center",
  },
  markAllText: {
    fontSize: 13,
    fontWeight: "600",
    color: theme.colors.accent,
  },
  listContent: {
    paddingBottom: theme.spacing.xxl,
    flexGrow: 1,
  },
  notifRow: {
    flexDirection: "row",
    backgroundColor: theme.colors.elevated,
    paddingVertical: 14,
    paddingHorizontal: theme.spacing.lg,
    borderBottomWidth: 0.5,
    borderBottomColor: `${theme.colors.text}10`,
  },
  notifRowUnread: {
    backgroundColor: "#FAFAF6",
  },
  unreadBorder: {
    position: "absolute",
    left: 0,
    top: 0,
    bottom: 0,
    width: 3,
  },
  dotContainer: {
    width: 24,
    paddingTop: theme.spacing.xs,
  },
  dot: {
    width: 8,
    height: 8,
    borderRadius: 4,
  },
  notifContent: {
    flex: 1,
  },
  notifTitle: {
    fontSize: 14,
    fontWeight: "500",
    color: theme.colors.text,
    marginBottom: 2,
  },
  notifTitleUnread: {
    fontWeight: "700",
  },
  notifBody: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.6,
    lineHeight: 18,
    marginBottom: theme.spacing.xs,
  },
  notifTime: {
    fontSize: 11,
    color: theme.colors.text,
    opacity: 0.35,
  },
  centered: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    paddingTop: 80,
  },
  emptyTitle: {
    fontSize: 16,
    fontWeight: "600",
    color: theme.colors.text,
    marginBottom: theme.spacing.xs,
  },
  emptySubtitle: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.5,
  },
});

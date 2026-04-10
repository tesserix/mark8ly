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
import type { Notification } from "@repo/mobile-shared/api/types";

const TYPE_COLORS: Record<string, string> = {
  order: "#2D4A2B",
  payment: "#F59E0B",
  alert: "#8B2020",
  system: "#0E0E0C",
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
  const dotColor = TYPE_COLORS[notification.type] ?? "#0E0E0C";
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
        >
          <Text style={styles.markAllText}>
            {markAllRead.isPending ? "Marking..." : "Mark all as read"}
          </Text>
        </TouchableOpacity>
      )}

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="large" color="#0E0E0C" />
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
              tintColor="#0E0E0C"
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
    backgroundColor: "#F7F6F2",
  },
  markAllBtn: {
    paddingHorizontal: 16,
    paddingVertical: 10,
    alignItems: "flex-end",
  },
  markAllText: {
    fontSize: 13,
    fontWeight: "600",
    color: "#2D4A2B",
  },
  listContent: {
    paddingBottom: 24,
    flexGrow: 1,
  },
  notifRow: {
    flexDirection: "row",
    backgroundColor: "#FFFFFF",
    paddingVertical: 14,
    paddingHorizontal: 16,
    borderBottomWidth: 0.5,
    borderBottomColor: "#0E0E0C10",
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
    paddingTop: 4,
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
    color: "#0E0E0C",
    marginBottom: 2,
  },
  notifTitleUnread: {
    fontWeight: "700",
  },
  notifBody: {
    fontSize: 13,
    color: "#0E0E0C",
    opacity: 0.6,
    lineHeight: 18,
    marginBottom: 4,
  },
  notifTime: {
    fontSize: 11,
    color: "#0E0E0C",
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
    color: "#0E0E0C",
    marginBottom: 4,
  },
  emptySubtitle: {
    fontSize: 13,
    color: "#0E0E0C",
    opacity: 0.5,
  },
});

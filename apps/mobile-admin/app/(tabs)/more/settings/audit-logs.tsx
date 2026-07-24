import { useCallback } from "react";
import { View, FlatList, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import { useAuditLogs } from "@/lib/hooks/use-audit-logs";
import { BackHeader, EmptyState, Hairline, Screen, StatusBadge, Text, type StatusTone } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { AuditLogEntry } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

const SEVERITY_TONE: Record<string, StatusTone> = {
  info: "muted",
  low: "muted",
  warning: "warning",
  medium: "warning",
  high: "danger",
  critical: "danger",
  error: "danger",
};

function titleize(value: string): string {
  return value
    .split(/[._]/)
    .map((w) => (w ? w.charAt(0).toUpperCase() + w.slice(1) : w))
    .join(" ");
}

function formatWhen(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("en-AU", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function LogRow({ entry }: { entry: AuditLogEntry }) {
  const who = entry.actor_type === "system" ? "System" : entry.user_email || "Unknown";
  const target = entry.resource_type ? ` · ${titleize(entry.resource_type)}` : "";
  return (
    <View style={styles.row}>
      <View style={styles.info}>
        <View style={styles.topRow}>
          <Text preset="bodyEmphasis" color="text" numberOfLines={1} style={styles.action}>
            {titleize(entry.action)}
            {target}
          </Text>
          {entry.severity && entry.severity !== "info" ? (
            <StatusBadge label={titleize(entry.severity)} tone={SEVERITY_TONE[entry.severity] ?? "muted"} />
          ) : null}
        </View>
        <Text preset="caption" color="textTertiary" numberOfLines={1}>
          {who} · {formatWhen(entry.timestamp)}
        </Text>
      </View>
    </View>
  );
}

export default function AuditLogsScreen() {
  const dockPad = useDockClearance();
  const { data, isLoading, isRefetching, isError, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } =
    useAuditLogs();

  const entries = data?.pages.flatMap((page) => page.data) ?? [];

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(
    ({ item, index }: { item: AuditLogEntry; index: number }) => (
      <View>
        {index > 0 ? <Hairline /> : null}
        <LogRow entry={item} />
      </View>
    ),
    [],
  );

  return (
    <Screen>
      <BackHeader eyebrow="SETTINGS" title="Audit log" />
      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && entries.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load audit logs"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <FlatList
          data={entries}
          renderItem={renderItem}
          keyExtractor={(item) => item.id}
          contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
          onEndReached={handleEndReached}
          onEndReachedThreshold={0.5}
          refreshControl={
            <RefreshControl refreshing={isRefetching} onRefresh={refetch} tintColor={theme.colors.text} />
          }
          ListFooterComponent={
            isFetchingNextPage ? (
              <View style={styles.footer}>
                <ActivityIndicator size="small" color={theme.colors.text} />
              </View>
            ) : null
          }
          ListEmptyComponent={
            <EmptyState title="No activity yet" message="Actions in your store show up here." />
          }
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  row: {
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.elevated,
  },
  info: { gap: 4 },
  topRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", gap: theme.spacing.sm },
  action: { flexShrink: 1 },
  list: { flexGrow: 1, paddingBottom: theme.spacing.huge },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
});

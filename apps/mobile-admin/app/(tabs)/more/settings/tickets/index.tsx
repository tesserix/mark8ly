import { useCallback, useState } from "react";
import {
  View,
  FlatList,
  TouchableOpacity,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { Plus, ChevronRight } from "lucide-react-native";
import { useTickets } from "@/lib/hooks/use-tickets";
import { BackHeader, EmptyState, Screen, SegmentedControl, StatusBadge, Text, type StatusTone } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Ticket } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "open" | "pending" | "resolved" | "closed";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "open", label: "Open" },
  { key: "pending", label: "Pending" },
  { key: "resolved", label: "Resolved" },
  { key: "closed", label: "Closed" },
];

export const TICKET_STATUS_TONE: Record<string, StatusTone> = {
  open: "warning",
  pending: "info",
  resolved: "success",
  closed: "muted",
};

export function titleizeStatus(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function TicketRow({ ticket, onPress }: { ticket: Ticket; onPress: (t: Ticket) => void }) {
  return (
    <TouchableOpacity
      style={styles.row}
      onPress={() => onPress(ticket)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`Ticket ${ticket.ticket_number}, ${ticket.status}`}
    >
      <View style={styles.info}>
        <View style={styles.topRow}>
          <Text preset="bodyEmphasis" color="text" numberOfLines={1} style={styles.subject}>
            {ticket.subject}
          </Text>
          <StatusBadge label={titleizeStatus(ticket.status)} tone={TICKET_STATUS_TONE[ticket.status] ?? "muted"} />
        </View>
        <Text preset="caption" color="textSecondary" numberOfLines={1}>
          #{ticket.ticket_number} · {ticket.submitted_by_name || ticket.submitted_by_email}
        </Text>
      </View>
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </TouchableOpacity>
  );
}

export default function TicketsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const [filter, setFilter] = useState<FilterKey>("all");

  const {
    data,
    isLoading,
    isRefetching,
    isError,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useTickets(filter !== "all" ? { status: filter } : undefined);

  const tickets = data?.pages.flatMap((page) => page.data) ?? [];

  const handlePress = useCallback(
    (ticket: Ticket) => router.push(`/(tabs)/more/settings/tickets/${ticket.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(
    ({ item }: { item: Ticket }) => <TicketRow ticket={item} onPress={handlePress} />,
    [handlePress],
  );

  return (
    <Screen>
      <BackHeader
        eyebrow="SETTINGS"
        title="Support tickets"
        rightSlot={
          <TouchableOpacity
            onPress={() => router.push("/(tabs)/more/settings/tickets/new")}
            hitSlop={12}
            accessibilityRole="button"
            accessibilityLabel="New ticket"
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </TouchableOpacity>
        }
      />
      <View style={styles.filter}>
        <SegmentedControl<FilterKey> segments={FILTERS} value={filter} onChange={setFilter} />
      </View>

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && tickets.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load tickets"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <FlatList
          data={tickets}
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
            <EmptyState
              title="No tickets"
              message={filter !== "all" ? "No tickets with this status." : "Customer support requests appear here."}
            />
          }
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  filter: { paddingTop: theme.spacing.sm },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
    gap: theme.spacing.md,
  },
  info: { flex: 1, gap: 4 },
  topRow: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm },
  subject: { flexShrink: 1 },
  list: { flexGrow: 1, paddingBottom: theme.spacing.huge },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
});

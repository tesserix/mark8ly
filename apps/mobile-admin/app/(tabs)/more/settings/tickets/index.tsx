import { useCallback, useMemo, useState } from "react";
import {
  View,
  Alert,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { Plus, ChevronRight, Archive } from "lucide-react-native";
import Animated, { FadeIn, useReducedMotion } from "react-native-reanimated";
import { useTickets } from "@/lib/hooks/use-tickets";
import { useUpdateTicketStatus } from "@/lib/admin-api/ticket-actions";
import {
  ActionFailureNotice,
  ActionSheet,
  CollapsingHeader,
  EmptyState,
  FilterChips,
  IconButton,
  PressableRow,
  Screen,
  StatusBadge,
  SwipeRow,
  Text,
  type ActionSheetItem,
  type StatusTone,
  type SwipeAction,
} from "@/components/ui";
import { useBusyIds } from "@/lib/use-busy-ids";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING } from "@/components/products/disclosure-motion";
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

const ICON_SIZE = 20;

export const TICKET_STATUS_TONE: Record<string, StatusTone> = {
  open: "warning",
  pending: "info",
  resolved: "success",
  closed: "muted",
};

export function titleizeStatus(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

/**
 * The one status a ticket cannot leave. `CanTransitionTo` returns false for
 * every target once a ticket is closed (ticket/models.go:129), and there is
 * no reopen endpoint anywhere — so this is the single value that decides
 * whether the row's gesture is armed at all.
 */
const TERMINAL_STATUS = "closed";

export function TicketRow({
  ticket,
  onPress,
  onLongPress,
}: {
  ticket: Ticket;
  onPress: (t: Ticket) => void;
  /**
   * Opens the row's long-press menu. OPTIONAL rather than always-supplied
   * because absence is the guard: the list screen passes `undefined` while
   * this row's own request is in flight. `SwipeRow.enabled` does NOT reach
   * this, which is why the two controls are gated separately.
   */
  onLongPress?: (t: Ticket) => void;
}) {
  return (
    <PressableRow
      lines={2}
      style={styles.row}
      onPress={() => onPress(ticket)}
      onLongPress={onLongPress ? () => onLongPress(ticket) : undefined}
      testID={`ticket-row-${ticket.id}`}
      accessibilityLabel={`Ticket ${ticket.ticket_number}, ${ticket.status}`}
      accessibilityHint={onLongPress ? "Long press for more actions" : undefined}
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
    </PressableRow>
  );
}

/**
 * Tickets — the support queue, worked with a thumb.
 *
 * 🔴 THE ONE SCREEN IN THIS INCREMENT WHERE THE ACTION IS NOT IDEMPOTENT.
 * `PATCH .../tickets/:id {status:"closed"}` on an already-closed ticket is
 * REFUSED: `CanTransitionTo` rejects a same-status target outright and
 * `closed` is terminal (ticket/models.go:112-133), and the resulting
 * `CodeInvalidTransition` maps to HTTP 409 (handlers/admin/errors.go:38).
 * There is no reopen endpoint. Two consequences shape everything here:
 *
 *  1. The gate is LOAD-BEARING, not cosmetic. On Products and Reviews a
 *     missing gate produces a pointless no-op; here it produces a visible
 *     server error on a row the merchant has no way to repair. A closed
 *     ticket therefore gets NO `SwipeRow` at all, and the menu's Close is
 *     `disabled` (kept, not dropped — `snapPoints` memoises on
 *     `items.length`).
 *  2. Close is CONFIRMED, even from the swipe. Everywhere else in this plan a
 *     revealed-then-tapped action needs no confirm because it is reversible.
 *     This one is not. One extra tap is the correct price for the only
 *     one-way action on the screen.
 *
 * CLOSE IS `neutral` ON THE SWIPE AND `danger` IN THE SHEET, deliberately.
 * Closing a resolved ticket is a normal outcome, not a destruction — and the
 * trailing edge is a POSITION, not a tone (the same reasoning Products' "Set
 * to draft" uses, and the row that proves the app-wide invariant test isn't
 * merely "trailing means danger"). In the sheet, where there is no side to
 * carry meaning, `danger` marks it as the one-way action.
 *
 * NO OPTIMISTIC HIDE and no optimistic update. `useUpdateTicketStatus`
 * invalidates ["tickets"], which prefix-matches this screen's own
 * ["tickets", "list", status] key, so the refetch is authoritative. It
 * deliberately does NOT invalidate ["dashboard"] — nothing in the dashboard
 * payload describes tickets, so that would be a refetch for nothing. That
 * exclusion is pinned in mutation-invalidations.test.tsx; leave it.
 */
export default function TicketsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const reduceMotion = useReducedMotion();
  const [filter, setFilter] = useState<FilterKey>("all");

  // Scroll offset the CollapsingHeader reads. Straight through, no rebase:
  // the chips are pinned above the list, so the list starts at 0.
  const { scrollY, onScroll: scrollHandler } = useCollapsingScroll();

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

  const tickets = useMemo(
    () => data?.pages.flatMap((page) => page.data) ?? [],
    [data],
  );

  const busy = useBusyIds();
  const updateStatus = useUpdateTicketStatus();
  // The ticket whose long-press menu is open. Also the only thing keeping the
  // menu mounted — `ActionSheet` is a controlled component.
  const [menuTicket, setMenuTicket] = useState<Ticket | null>(null);

  const handlePress = useCallback(
    (ticket: Ticket) => router.push(`/(tabs)/more/settings/tickets/${ticket.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const closeTicket = useCallback(
    (ticket: Ticket) => {
      busy.markBusy(ticket.id);
      updateStatus.mutate(
        { id: ticket.id, status: TERMINAL_STATUS },
        // The action label matters more here than on any other screen in the
        // increment: a 409 is a refusal the merchant can genuinely hit, and
        // without a message it is a haptic and a badge that still reads Open.
        busy.settleCallbacks(ticket.id, "close this ticket"),
      );
    },
    // The two STABLE callbacks off `busy`, not the whole object: its identity
    // changes on every busy transition, so depending on it would re-derive
    // this callback — and therefore every row's actions — each time a
    // mutation starts or settles.
    [updateStatus, busy.markBusy, busy.settleCallbacks],
  );

  /**
   * The confirm. This is the ONLY action in the increment that carries one on
   * a revealed-then-tapped swipe, because it is the only one with no way
   * back — the copy says so, and that sentence is asserted.
   */
  const confirmClose = useCallback(
    (ticket: Ticket) => {
      Alert.alert(
        "Close this ticket?",
        `Ticket #${ticket.ticket_number} will be closed. Closed tickets cannot be reopened — the customer would have to raise a new one.`,
        [
          // No `onPress`: cancelling must be inert, not a no-op handler that
          // could later grow a body.
          { text: "Cancel", style: "cancel" },
          { text: "Close ticket", style: "destructive", onPress: () => closeTicket(ticket) },
        ],
      );
    },
    [closeTicket],
  );

  /**
   * Swipe actions for one ticket, or `null` for a closed one — the caller
   * mounts no `SwipeRow` at all in that case.
   *
   * There is no LEADING action by design: nothing on this screen adds
   * anything to a ticket, so the constructive edge stays unarmed rather than
   * being filled for symmetry.
   */
  const actionsFor = useCallback(
    (ticket: Ticket): { leading: SwipeAction[]; trailing: SwipeAction[] } | null => {
      if (ticket.status === TERMINAL_STATUS) return null;
      return {
        leading: [],
        trailing: [
          {
            key: "close",
            label: "Close",
            // Dismissive, not destructive — see the screen doc comment. The
            // one-way-ness is carried by the confirm, not by the paint.
            tone: "neutral",
            icon: <Archive size={ICON_SIZE} color={theme.colors.text} strokeWidth={2} />,
            onPress: () => confirmClose(ticket),
          },
        ],
      };
    },
    [confirmClose],
  );

  /**
   * The long-press menu. ALWAYS these two items, in this order — the illegal
   * one is `disabled` rather than dropped, because `ActionSheet`'s
   * `snapPoints` memoises on `items.length` and a dropped item resizes the
   * sheet under the merchant's thumb.
   *
   * "Assign" is CUT: there is no assignee model and no route for it.
   */
  const menuItems = useMemo((): ActionSheetItem[] => {
    const target = menuTicket;
    if (!target) return [];
    return [
      {
        key: "reply",
        label: "Reply",
        // Navigation, not a mutation: the reply body is required and typed on
        // the detail screen.
        onPress: () => handlePress(target),
      },
      {
        key: "close",
        label: "Close",
        // `danger` HERE and `neutral` on the swipe — see the screen doc
        // comment. No side carries meaning in a sheet, so the tone has to.
        tone: "danger",
        disabled: target.status === TERMINAL_STATUS,
        onPress: () => confirmClose(target),
      },
    ];
  }, [menuTicket, handlePress, confirmClose]);

  const renderItem = useCallback(
    ({ item }: { item: Ticket }) => {
      const actions = actionsFor(item);
      const row = (
        <TicketRow
          ticket={item}
          onPress={handlePress}
          // Gated on the SAME busy set as the swipe below: the menu is a
          // second route onto the row, and `SwipeRow.enabled` does not reach
          // this handler.
          onLongPress={busy.isBusy(item.id) ? undefined : setMenuTicket}
        />
      );
      // A closed ticket gets NO gesture container at all — see `actionsFor`.
      if (actions === null) return row;
      return (
        <SwipeRow
          testID={`swipe-${item.id}`}
          leadingActions={actions.leading}
          trailingActions={actions.trailing}
          // Suppressed while THIS row's own request is open, so a
          // still-visible row can't be fired at twice.
          enabled={!busy.isBusy(item.id)}
        >
          {row}
        </SwipeRow>
      );
    },
    [actionsFor, handlePress, busy.isBusy],
  );

  return (
    <Screen>
      <CollapsingHeader
        eyebrow="SUPPORT"
        title="Tickets"
        // Nested route under More → Settings. The chevron gets its OWN nav
        // row when expanded, so the eyebrow, title, chips and rows all share
        // gutter 20.
        onBack={() => router.back()}
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/settings/tickets/new")}
            accessibilityLabel="New ticket"
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </IconButton>
        }
        scrollY={scrollY}
      />
      {/* Pills, matching Orders. Pinned ABOVE the list, never in
          `ListHeaderComponent`. Semantics untouched — `all` sends no
          `status`, every other key IS the `status` value. */}
      <FilterChips<FilterKey> chips={FILTERS} value={filter} onChange={setFilter} />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && tickets.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load tickets"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.View
          testID="tickets-list-wrap"
          style={styles.listWrap}
          entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}
        >
          <Animated.FlatList
            testID="tickets-list"
            data={tickets}
            renderItem={renderItem}
            keyExtractor={(item) => (item as Ticket).id}
            onScroll={scrollHandler}
            scrollEventThrottle={16}
            style={styles.listFlex}
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
                align="left"
                title="No tickets"
                message={filter !== "all" ? "No tickets with this status." : "Customer support requests appear here."}
              />
            }
          />
        </Animated.View>
      )}

      {/* Arbitrary merchant/customer text — clamped to one line by
          `ActionSheet`, whose height budget reserves exactly that many. */}
      <ActionSheet
        title={menuTicket?.subject}
        items={menuItems}
        visible={menuTicket !== null}
        onDismiss={() => setMenuTicket(null)}
      />

      {/* Why the last close changed nothing — the 409 this screen's gate
          exists to avoid is the one refusal a merchant can genuinely hit. */}
      <ActionFailureNotice failure={busy.failure} onDismiss={busy.dismissFailure} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  row: {
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
  },
  info: { flex: 1, gap: 4 },
  topRow: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm },
  subject: { flexShrink: 1 },
  listWrap: { flex: 1 },
  listFlex: { flex: 1 },
  list: { flexGrow: 1, paddingBottom: theme.spacing.huge },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  // NOT `styles.centered` for the error state: that wrapper's
  // `alignItems: "center"` shrink-wraps and re-centres its child, silently
  // undoing `EmptyState align="left"`. Claims the remaining height only.
  errorSlot: { flex: 1 },
});

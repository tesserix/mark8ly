import { useCallback, useMemo, useState } from "react";
import { View, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import Animated from "react-native-reanimated";
import { useRouter } from "expo-router";
import { Check, Pause, Plus } from "lucide-react-native";
import { useGiftCards } from "@/lib/hooks/use-gift-cards";
import { GiftCardRow } from "@/components/marketing/GiftCardRow";
import {
  ActionFailureNotice,
  ActionSheet,
  CollapsingHeader,
  EmptyState,
  FilterChips,
  IconButton,
  Screen,
  SwipeRow,
  type ActionSheetItem,
  type SwipeAction,
} from "@/components/ui";
import { useSetGiftCardStatus } from "@/lib/admin-api/gift-card-actions";
import {
  canSetGiftCardStatus,
  type GiftCardStatusTarget,
} from "@/lib/gift-card-status";
import { useBusyIds } from "@/lib/use-busy-ids";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
import { theme } from "@/lib/theme";
import type { GiftCard } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "active" | "redeemed" | "expired" | "disabled";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  { key: "redeemed", label: "Redeemed" },
  { key: "expired", label: "Expired" },
  { key: "disabled", label: "Off" },
];

const ICON_SIZE = 20;

/**
 * What the merchant tried, read after "Couldn't " in the failure notice.
 *
 * Keyed off the SAME union the mutation takes, so a third target could never
 * reach the server with no name to report when it fails.
 */
const ACTION_FOR_TARGET: Record<GiftCardStatusTarget, string> = {
  disabled: "disable this gift card",
  active: "enable this gift card",
};

/**
 * Gift cards — the ledger, worked with a thumb.
 *
 * Three affordances, exactly as Products and Orders:
 *
 *  1. Swipe right → Enable (a disabled card). Swipe left → Disable (an
 *     active one). Constructive at the leading edge in moss, per the
 *     app-wide convention. The trailing action is `neutral`, NOT danger:
 *     disabling a gift card is dismissive, reversible and idempotent, and
 *     the trailing edge is a POSITION not a tone. Neither fires on the drag
 *     — `SwipeRow` settles open and the revealed button is tapped; nothing
 *     in this app opts into `autoFireOnFullSwipe` because there is no undo.
 *  2. Long press → the three-action menu, always three items so the sheet
 *     never resizes under the thumb (see `menuItems`).
 *  3. Tap → the card detail and its transaction ledger.
 *
 * **What Disable means, and what it does not.** A merchant may disable a
 * card a customer has already paid for and holds a balance on. The safeguard
 * is REVERSIBILITY, not restriction: disabling freezes the remaining
 * balance, and enabling restores it in full. No refund is issued and no
 * balance is ever destroyed, which is why there is no confirm dialog in
 * front of it and why the copy on both actions names the balance rather than
 * the card. Nothing here may be worded so as to suggest the money is gone.
 *
 * **DELETE IS CUT, permanently.** The backend exposes none, and one would
 * CASCADE the gift_card_transactions ledger — including rows that reference
 * real orders. Do not add it.
 *
 * A card in any other state gets NO `SwipeRow` at all: `pending`, `depleted`
 * and `refunded` are 409 `invalid_transition`, and any card past its
 * `expires_at` is 410 `gift_card_expired` in both directions. An armed
 * gesture that can only 4xx is worse than no gesture — the same rule that
 * gives a terminal order no `SwipeRow` on Orders. In the menu those items
 * are `disabled` rather than dropped, so the sheet keeps its height.
 *
 * NO OPTIMISTIC HIDE, and no optimistic update. `useSetGiftCardStatus`
 * invalidates ["gift-cards"], a strict prefix of this screen's own
 * ["gift-cards", "list", status] key, so the refetch is authoritative: under
 * a status filter the actioned card drops out by itself, and under All its
 * badge simply flips. What the absent hide leaves open — a second action on
 * a row whose request is still in flight — is closed by disabling THAT row's
 * gesture AND its long-press (`useBusyIds`), a guard on the control keyed
 * purely on the mutation lifecycle.
 */
export default function GiftCardsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const { scrollY, onScroll } = useCollapsingScroll();
  const [filter, setFilter] = useState<FilterKey>("all");
  const busy = useBusyIds();
  const setStatus = useSetGiftCardStatus();
  // The card whose long-press menu is open. Also the only thing keeping the
  // menu mounted — `ActionSheet` is a controlled component.
  const [menuCard, setMenuCard] = useState<GiftCard | null>(null);

  const {
    data,
    isLoading,
    isRefetching,
    isError,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useGiftCards(filter !== "all" ? { status: filter } : undefined);

  const cards = data?.pages.flatMap((page) => page.data) ?? [];

  const handlePress = useCallback(
    (card: GiftCard) => router.push(`/(tabs)/more/marketing/gift-cards/${card.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const setCardStatus = useCallback(
    (card: GiftCard, status: GiftCardStatusTarget) => {
      busy.markBusy(card.id);
      setStatus.mutate(
        { id: card.id, status },
        // The action label is what turns a bare failure haptic into
        // "Couldn't disable this gift card — <the server's reason>". Without
        // it the merchant is left staring at an unchanged badge.
        busy.settleCallbacks(card.id, ACTION_FOR_TARGET[status]),
      );
    },
    // The two STABLE callbacks off `busy`, not the whole object: its identity
    // changes on every busy transition, so depending on it would re-derive
    // this callback — and therefore every row's actions — each time a
    // mutation starts or settles.
    [setStatus, busy.markBusy, busy.settleCallbacks],
  );

  /**
   * Swipe actions for one card, or `null` when neither transition is legal.
   *
   * `null` means NO `SwipeRow` is mounted at all, not an empty one: an empty
   * `SwipeRow` still arms a pan gesture that swallows part of the drag and
   * reveals nothing, which reads as a broken row rather than an inert one.
   */
  const actionsFor = useCallback(
    (card: GiftCard): { leading: SwipeAction[]; trailing: SwipeAction[] } | null => {
      const canEnable = canSetGiftCardStatus(card, "active");
      const canDisable = canSetGiftCardStatus(card, "disabled");
      if (!canEnable && !canDisable) return null;

      const leading: SwipeAction[] = canEnable
        ? [
            {
              key: "enable",
              label: "Enable",
              tone: "accent",
              icon: <Check size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
              onPress: () => setCardStatus(card, "active"),
            },
          ]
        : [];

      const trailing: SwipeAction[] = canDisable
        ? [
            {
              key: "disable",
              // The 84pt action panel takes one short word; the sentence that
              // explains what freezing means lives on the menu item.
              label: "Disable",
              // Dismissive, not destructive — see the screen doc comment.
              tone: "neutral",
              icon: <Pause size={ICON_SIZE} color={theme.colors.text} strokeWidth={2} />,
              onPress: () => setCardStatus(card, "disabled"),
            },
          ]
        : [];

      return { leading, trailing };
    },
    [setCardStatus],
  );

  /**
   * The long-press menu. ALWAYS these three items, in this order — illegal
   * ones are `disabled` rather than dropped, because `ActionSheet`'s
   * `snapPoints` memoises on `items.length` and a dropped item resizes the
   * sheet under the merchant's thumb.
   *
   * The two toggle labels are where the product promise is written down: a
   * merchant deciding whether to disable a card a customer has paid for must
   * be able to read, at the moment of deciding, that the balance is being
   * FROZEN and can be given back in full. "Disable"/"Enable" alone would
   * leave them guessing whether they are about to confiscate someone's
   * money.
   */
  const menuItems = useMemo((): ActionSheetItem[] => {
    const target = menuCard;
    if (!target) return [];
    return [
      { key: "view", label: "View", onPress: () => handlePress(target) },
      {
        key: "enable",
        label: "Enable — restores the balance",
        disabled: !canSetGiftCardStatus(target, "active"),
        onPress: () => setCardStatus(target, "active"),
      },
      {
        key: "disable",
        label: "Disable — freezes the balance",
        // NOT `tone: "danger"`. Nothing is destroyed and the merchant can
        // walk it back with the same thumb motion.
        disabled: !canSetGiftCardStatus(target, "disabled"),
        onPress: () => setCardStatus(target, "disabled"),
      },
    ];
  }, [menuCard, handlePress, setCardStatus]);

  const renderItem = useCallback(
    ({ item }: { item: GiftCard }) => {
      const row = (
        <GiftCardRow
          card={item}
          onPress={handlePress}
          // Gated on the SAME busy set as the swipe below: the menu is an
          // independent second route onto the row, and `SwipeRow.enabled`
          // does not reach this handler.
          onLongPress={busy.isBusy(item.id) ? undefined : setMenuCard}
        />
      );
      const actions = actionsFor(item);
      if (!actions) return row;
      return (
        <SwipeRow
          testID={`swipe-${item.id}`}
          leadingActions={actions.leading}
          trailingActions={actions.trailing}
          // Suppressed while THIS row's own request is open, so a
          // still-visible row can't be fired at twice. Not a claim about the
          // data — see the screen's doc comment.
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
        eyebrow="MARKETING"
        title="Gift cards"
        // This is a NESTED route — it used `BackHeader`, which had a chevron
        // this primitive did not. `onBack` is the additive prop that closed
        // that gap; without it the merchant would be stranded here.
        onBack={() => router.back()}
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/marketing/gift-cards/new")}
            accessibilityLabel="Issue gift card"
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </IconButton>
        }
        scrollY={scrollY}
      />
      {/* Pills, matching Orders. Pinned OUTSIDE the list and above it — not
          in `ListHeaderComponent`. `FilterChips` owns its own vertical rhythm
          and a hugging wrapper; inside the list that wrapper stretches and
          leaves ~110pt of dead paper between the header and the first row.
          Semantics untouched — `all` sends no `status`, every other key IS
          the `status` value. */}
      <FilterChips<FilterKey> chips={FILTERS} value={filter} onChange={setFilter} />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && cards.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load gift cards"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.FlatList
          testID="gift-cards-list"
          data={cards}
          renderItem={renderItem}
          keyExtractor={(item) => (item as GiftCard).id}
          // The pair that makes the header collapse at all: a plain FlatList
          // has nothing to feed `scrollY` and would freeze it expanded.
          onScroll={onScroll}
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
              title="No gift cards yet"
              message={
                filter !== "all"
                  ? "No gift cards with this status."
                  : "Issue a gift card for a customer."
              }
            />
          }
        />
      )}

      <ActionSheet
        title={menuCard?.code_display}
        items={menuItems}
        visible={menuCard !== null}
        onDismiss={() => setMenuCard(null)}
      />

      {/* Why the last swipe changed nothing. Floats above the dock and
          replaces itself, so a merchant firing several rows gets one
          readable message rather than a stack. No `bottomOffset`: unlike
          Products, nothing else floats at this screen's bottom edge. */}
      <ActionFailureNotice failure={busy.failure} onDismiss={busy.dismissFailure} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  // Explicit flex, not leftover space: the list is the only child of `Screen`
  // that should absorb the remaining height. Same as Orders.
  listFlex: { flex: 1 },
  // `paddingBottom` comes from `useDockClearance` at the call site — the old
  // hard-coded `theme.spacing.huge` here was a second, smaller answer to the
  // same question and lost to the inline one anyway.
  list: { flexGrow: 1 },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  // NOT `styles.centered` for the error state: that wrapper's
  // `alignItems: "center"` shrink-wraps and re-centres its child, silently
  // undoing `EmptyState align="left"`. Claims the remaining height only.
  errorSlot: { flex: 1 },
});

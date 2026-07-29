import { useCallback, useMemo, useState } from "react";
import {
  View,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { Check, X } from "lucide-react-native";
import Animated, { FadeIn, useReducedMotion } from "react-native-reanimated";
import { useReviews } from "../../../../lib/hooks/use-reviews";
import { useApproveReview, useRejectReview } from "@/lib/admin-api/review-actions";
import { ReviewRow } from "../../../../components/reviews/ReviewRow";
import {
  ActionFailureNotice,
  ActionSheet,
  CollapsingHeader,
  EmptyState,
  FilterChips,
  Screen,
  SwipeRow,
  type ActionSheetItem,
  type SwipeAction,
} from "@/components/ui";
import { useBusyIds } from "@/lib/use-busy-ids";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING } from "@/components/products/disclosure-motion";
import type { Review } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "pending" | "approved" | "rejected";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "pending", label: "Pending" },
  { key: "approved", label: "Approved" },
  { key: "rejected", label: "Rejected" },
];

const ICON_SIZE = 20;

/**
 * Reviews — the moderation queue, worked with a thumb.
 *
 * Three affordances, in increasing order of commitment, exactly as Products:
 *
 *  1. Swipe right → Approve. Swipe left → Reject. Per the app-wide
 *     convention, constructive at the leading edge in moss, the negative
 *     outcome trailing in oxblood. Neither fires on the drag — `SwipeRow`
 *     settles open and the revealed button is tapped.
 *  2. Long press → the three-action menu, always three items so the sheet
 *     never resizes under the thumb (see `menuItems`).
 *  3. Tap → the review detail, where the reply box lives.
 *
 * NEITHER ACTION IS BEHIND A CONFIRM, and Reject's danger tone is not an
 * oversight. `POST /reviews/:id/approve` and `/reject` take no body and are
 * explicitly idempotent, and each is reversible by firing the other — so a
 * confirm would tax the most common triage action in the app for no safety.
 * The tone says which OUTCOME the gesture picks for the customer's review,
 * not how final it is; Products' neutral "Set to draft" makes the same
 * distinction from the other side.
 *
 * A NON-PENDING review gets no `SwipeRow` at all, matching Orders'
 * terminal-status rule: an armed gesture that can only re-assert a decision
 * already made is worse than no gesture, and it teaches the wrong thing about
 * the row. The menu still reaches both actions, so nothing becomes
 * unreachable — it is only the one-thumb shortcut that is withdrawn.
 *
 * NO OPTIMISTIC HIDE and no optimistic update. `useApproveReview` and
 * `useRejectReview` invalidate ["reviews"], which prefix-matches this
 * screen's own ["reviews", status] key, so the refetch is authoritative about
 * this list's contents: under the Pending chip the moderated review drops out
 * by itself, and under All its badge simply flips. They also invalidate
 * ["dashboard"] — the ONE cross-screen invalidation in this increment,
 * because the dashboard payload carries its own `stats.pending_reviews` and
 * drives the queue's "See all N pending reviews" row. Do not add or remove an
 * invalidation there.
 *
 * What the absent hide leaves open — a second action on a row whose request
 * is still in flight — is closed by disabling THAT row's gesture AND its
 * long-press (`useBusyIds`). A guard on the control, keyed purely on the
 * mutation lifecycle.
 */
export default function ReviewsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const reduceMotion = useReducedMotion();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");

  // Scroll offset the CollapsingHeader reads. Straight through, no rebase:
  // the chips are pinned above the list, so the list starts at 0 and the
  // header's resting state and the list's resting position already agree.
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
  } = useReviews(activeFilter !== "all" ? { status: activeFilter } : undefined);

  const reviews = useMemo(
    () => data?.pages.flatMap((page) => page.data) ?? [],
    [data],
  );

  const busy = useBusyIds();
  const approveReview = useApproveReview();
  const rejectReview = useRejectReview();
  // The review whose long-press menu is open. Also the only thing keeping the
  // menu mounted — `ActionSheet` is a controlled component.
  const [menuReview, setMenuReview] = useState<Review | null>(null);

  const handlePress = useCallback(
    (review: Review) => router.push(`/(tabs)/customers/reviews/${review.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const approve = useCallback(
    (review: Review) => {
      busy.markBusy(review.id);
      // The action label is what turns a bare failure haptic into "Couldn't
      // approve this review — <the server's reason>". Without it a failed
      // moderation and a successful one look identical: the badge simply
      // stays where it was.
      approveReview.mutate(review.id, busy.settleCallbacks(review.id, "approve this review"));
    },
    // The two STABLE callbacks off `busy`, not the whole object: its identity
    // changes on every busy transition, so depending on it would re-derive
    // this callback — and therefore every row's actions — each time a
    // mutation starts or settles.
    [approveReview, busy.markBusy, busy.settleCallbacks],
  );

  const reject = useCallback(
    (review: Review) => {
      busy.markBusy(review.id);
      rejectReview.mutate(review.id, busy.settleCallbacks(review.id, "reject this review"));
    },
    [rejectReview, busy.markBusy, busy.settleCallbacks],
  );

  /**
   * Swipe actions for one review, or `null` for a review that has already
   * been moderated — the caller mounts no `SwipeRow` at all in that case.
   *
   * Both transitions remain LEGAL server-side on a decided review (they are
   * idempotent, and approve↔reject is a supported flip), so this gate exists
   * for the merchant rather than the API. The menu keeps both actions
   * reachable; only the gesture is withdrawn.
   */
  const actionsFor = useCallback(
    (review: Review): { leading: SwipeAction[]; trailing: SwipeAction[] } | null => {
      if (review.status !== "pending") return null;
      return {
        leading: [
          {
            key: "approve",
            label: "Approve",
            tone: "accent",
            icon: <Check size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
            onPress: () => approve(review),
          },
        ],
        trailing: [
          {
            key: "reject",
            label: "Reject",
            // Danger because it is the negative outcome for the customer's
            // review — NOT because it is irreversible. It fires directly.
            tone: "danger",
            icon: <X size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
            onPress: () => reject(review),
          },
        ],
      };
    },
    [approve, reject],
  );

  /**
   * The long-press menu. ALWAYS these three items, in this order — illegal
   * ones are `disabled` rather than dropped, because `ActionSheet`'s
   * `snapPoints` memoises on `items.length` and a dropped item resizes the
   * sheet under the merchant's thumb.
   *
   * "Report" is CUT: no endpoint exists for it. Three items, constant length.
   */
  const menuItems = useMemo((): ActionSheetItem[] => {
    const target = menuReview;
    if (!target) return [];
    return [
      {
        key: "reply",
        label: "Reply",
        // Navigation, not a mutation: the reply body is required and typed on
        // the detail screen, so a bare tap here could only ever produce a 400.
        onPress: () => handlePress(target),
      },
      {
        key: "approve",
        label: "Approve",
        disabled: target.status === "approved",
        onPress: () => approve(target),
      },
      {
        key: "reject",
        label: "Reject",
        tone: "danger",
        disabled: target.status === "rejected",
        onPress: () => reject(target),
      },
    ];
  }, [menuReview, handlePress, approve, reject]);

  const renderItem = useCallback(
    ({ item }: { item: Review }) => {
      const actions = actionsFor(item);
      const row = (
        <ReviewRow
          review={item}
          onPress={handlePress}
          // Gated on the SAME busy set as the swipe below: the menu is a
          // second route onto the row, and `SwipeRow.enabled` does not reach
          // this handler.
          onLongPress={busy.isBusy(item.id) ? undefined : setMenuReview}
        />
      );
      // A moderated review gets NO gesture container at all — see `actionsFor`.
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
        eyebrow="CUSTOMERS"
        title="Reviews"
        // Nested route, reached from the Customers header link. The chevron
        // gets its OWN nav row when expanded, so the eyebrow, title, chips
        // and rows all share gutter 20.
        onBack={() => router.back()}
        scrollY={scrollY}
      />
      {/* Pills, matching Orders. Pinned ABOVE the list, never in
          `ListHeaderComponent` — that re-introduces the dead-paper bug.
          Semantics untouched — `all` sends no `status`, every other key IS
          the `status` value. */}
      <FilterChips<FilterKey>
        chips={FILTERS}
        value={activeFilter}
        onChange={setActiveFilter}
      />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && reviews.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load reviews"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.View
          testID="reviews-list-wrap"
          style={styles.listWrap}
          entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}
        >
          <Animated.FlatList
            testID="reviews-list"
            data={reviews}
            renderItem={renderItem}
            keyExtractor={(item) => (item as Review).id}
            onScroll={scrollHandler}
            scrollEventThrottle={16}
            style={styles.listFlex}
            contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
            onEndReached={handleEndReached}
            onEndReachedThreshold={0.5}
            refreshControl={
              <RefreshControl
                refreshing={isRefetching}
                onRefresh={refetch}
                tintColor={theme.colors.text}
              />
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
                title="No reviews yet"
                message={
                  activeFilter !== "all"
                    ? "No reviews with this status."
                    : "Customer reviews appear here for moderation."
                }
              />
            }
          />
        </Animated.View>
      )}

      {/* Arbitrary customer text — clamped to one line by `ActionSheet`, whose
          height budget reserves exactly that many lines. */}
      <ActionSheet
        title={menuReview ? menuReview.title || menuReview.content : undefined}
        items={menuItems}
        visible={menuReview !== null}
        onDismiss={() => setMenuReview(null)}
      />

      {/* Why the last swipe changed nothing. Floats above the dock and
          replaces itself, so a merchant moderating several rows gets one
          readable message rather than a stack. */}
      <ActionFailureNotice failure={busy.failure} onDismiss={busy.dismissFailure} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  listWrap: { flex: 1 },
  listFlex: { flex: 1 },
  list: {
    flexGrow: 1,
    paddingBottom: theme.spacing.huge,
  },
  footer: {
    paddingVertical: theme.spacing.lg,
    alignItems: "center",
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
  // NOT `styles.centered` for the error state: that wrapper's
  // `alignItems: "center"` shrink-wraps and re-centres its child, silently
  // undoing `EmptyState align="left"`. Claims the remaining height only.
  errorSlot: { flex: 1 },
});

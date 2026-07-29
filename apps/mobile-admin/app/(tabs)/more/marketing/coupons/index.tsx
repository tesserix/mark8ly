import { useCallback, useMemo, useState } from "react";
import {
  View,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { Plus, Check, Ban } from "lucide-react-native";
import Animated, { FadeIn, useReducedMotion } from "react-native-reanimated";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useCoupons } from "@/lib/hooks/use-coupons";
import { usePatchCoupon } from "@/lib/admin-api/coupon-actions";
import { CouponRow } from "@/components/marketing/CouponRow";
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
import { useBusyIds } from "@/lib/use-busy-ids";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING } from "@/components/products/disclosure-motion";
import type { Coupon } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "active" | "scheduled" | "expired" | "disabled";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  { key: "scheduled", label: "Scheduled" },
  { key: "expired", label: "Expired" },
  { key: "disabled", label: "Off" },
];

const ICON_SIZE = 20;

/**
 * The only two statuses a merchant may set from a row.
 *
 * `scheduled` and `expired` are DERIVED from the coupon's own date window —
 * they are not states you assign, they are states the calendar puts a coupon
 * in — so neither is a legal target. The backend agrees and says so first:
 * `Service.Patch` rejects any status but these two before it even loads the
 * row, with `status must be 'active' or 'disabled'`
 * (coupon/service.go:159-169). So unlike Products — where a missing gate is
 * only a pointless no-op — an ungated row here would put a validation error
 * in front of the merchant.
 */
type ToggleStatus = "active" | "disabled";

/** What the merchant tried, read after "Couldn't " in the failure notice. */
const ACTION_FOR_STATUS: Record<ToggleStatus, string> = {
  active: "switch this coupon on",
  disabled: "switch this coupon off",
};

/**
 * Coupons — the discount list, worked with a thumb.
 *
 * Three affordances, in increasing order of commitment, exactly as Products:
 *
 *  1. Swipe right → Enable. Swipe left → Disable. Per the app-wide
 *     convention, constructive at the leading edge in moss. The trailing
 *     action is `neutral`, NOT danger: switching a code off is dismissive,
 *     reversible and idempotent, and the trailing edge is a POSITION not a
 *     tone. Neither fires on the drag.
 *  2. Long press → the three-action menu, always three items so the sheet
 *     never resizes under the thumb (see `menuItems`).
 *  3. Tap → the coupon detail, where everything is editable.
 *
 * AN EXPIRED OR SCHEDULED COUPON GETS NO `SwipeRow` AT ALL. Both statuses are
 * derived from the date window rather than assigned, so neither toggle is a
 * transition the merchant can meaningfully make from a row — and an armed
 * gesture that can only mislead is worse than no gesture. The menu keeps both
 * items present but `disabled`, so the sheet's height never changes.
 *
 * "DELETE" IS DELIBERATELY ABSENT, and that is a correctness decision rather
 * than a scope one. `DELETE /coupons/:id` does not delete: it calls
 * `SoftDisable`, returns `200 {"message":"coupon disabled"}`
 * (handlers/admin/coupons.go:259), logs the audit action as a deactivation,
 * and LEAVES THE ROW IN THE LIST. An item labelled "Delete" that leaves the
 * coupon on screen is a lie to the merchant, and "Disable" — the honest
 * endpoint for exactly that outcome — is already offered above it.
 * "Duplicate" is cut for the ordinary reason: no endpoint.
 *
 * NO OPTIMISTIC HIDE and no optimistic update. `usePatchCoupon` invalidates
 * ["coupons"], which prefix-matches this screen's own
 * ["coupons", "list", status, search] key, so the refetch is authoritative
 * about this list's contents: under a status filter the toggled coupon drops
 * out by itself, and under All its badge simply flips.
 *
 * What the absent hide leaves open — a second action on a row whose request
 * is still in flight — is closed by disabling THAT row's gesture AND its
 * long-press (`useBusyIds`).
 */
export default function CouponsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const reduceMotion = useReducedMotion();
  const currency = useTenantStore((s) => s.activeStore?.currency_code) || "AUD";
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
  } = useCoupons(filter !== "all" ? { status: filter } : undefined);

  const coupons = useMemo(
    () => data?.pages.flatMap((page) => page.data) ?? [],
    [data],
  );

  const busy = useBusyIds();
  const patchCoupon = usePatchCoupon();
  // The coupon whose long-press menu is open. Also the only thing keeping the
  // menu mounted — `ActionSheet` is a controlled component.
  const [menuCoupon, setMenuCoupon] = useState<Coupon | null>(null);

  const handlePress = useCallback(
    (coupon: Coupon) => router.push(`/(tabs)/more/marketing/coupons/${coupon.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const setCouponStatus = useCallback(
    (coupon: Coupon, status: ToggleStatus) => {
      busy.markBusy(coupon.id);
      patchCoupon.mutate(
        { id: coupon.id, body: { status } },
        // The action label is what turns a bare failure haptic into
        // "Couldn't switch this coupon off — <the server's reason>".
        busy.settleCallbacks(coupon.id, ACTION_FOR_STATUS[status]),
      );
    },
    // The two STABLE callbacks off `busy`, not the whole object: its identity
    // changes on every busy transition, so depending on it would re-derive
    // this callback — and therefore every row's actions — each time a
    // mutation starts or settles.
    [patchCoupon, busy.markBusy, busy.settleCallbacks],
  );

  /**
   * Swipe actions for one coupon, or `null` when neither transition is legal
   * — the caller mounts no `SwipeRow` at all in that case.
   *
   * Exactly ONE action is ever offered, because the two are mutually
   * exclusive: an active coupon can only be switched off, a disabled one can
   * only be switched on. Anything else (scheduled, expired) gets neither.
   */
  const actionsFor = useCallback(
    (coupon: Coupon): { leading: SwipeAction[]; trailing: SwipeAction[] } | null => {
      if (coupon.status === "disabled") {
        return {
          leading: [
            {
              key: "enable",
              label: "Enable",
              tone: "accent",
              icon: <Check size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
              onPress: () => setCouponStatus(coupon, "active"),
            },
          ],
          trailing: [],
        };
      }
      if (coupon.status === "active") {
        return {
          leading: [],
          trailing: [
            {
              key: "disable",
              label: "Disable",
              // Dismissive, not destructive — see the screen doc comment.
              tone: "neutral",
              icon: <Ban size={ICON_SIZE} color={theme.colors.text} strokeWidth={2} />,
              onPress: () => setCouponStatus(coupon, "disabled"),
            },
          ],
        };
      }
      // scheduled | expired — derived states, no legal toggle.
      return null;
    },
    [setCouponStatus],
  );

  /**
   * The long-press menu. ALWAYS these three items, in this order — illegal
   * ones are `disabled` rather than dropped, because `ActionSheet`'s
   * `snapPoints` memoises on `items.length` and a dropped item resizes the
   * sheet under the merchant's thumb. On an EXPIRED coupon both toggles grey
   * out and the sheet keeps its height.
   *
   * There is no Delete and no Duplicate — see the screen doc comment.
   */
  const menuItems = useMemo((): ActionSheetItem[] => {
    const target = menuCoupon;
    if (!target) return [];
    return [
      {
        key: "edit",
        label: "Edit",
        // Stays live on every status: an expired coupon's date window is
        // exactly what a merchant would go in to change.
        onPress: () => handlePress(target),
      },
      {
        key: "enable",
        label: "Enable",
        disabled: target.status !== "disabled",
        onPress: () => setCouponStatus(target, "active"),
      },
      {
        key: "disable",
        label: "Disable",
        // No `danger` tone anywhere in this sheet: both toggles are
        // reversible, so nothing here has earned oxblood.
        disabled: target.status !== "active",
        onPress: () => setCouponStatus(target, "disabled"),
      },
    ];
  }, [menuCoupon, handlePress, setCouponStatus]);

  const renderItem = useCallback(
    ({ item }: { item: Coupon }) => {
      const actions = actionsFor(item);
      const row = (
        <CouponRow
          coupon={item}
          currency={currency}
          onPress={handlePress}
          // Gated on the SAME busy set as the swipe below: the menu is a
          // second route onto the row, and `SwipeRow.enabled` does not reach
          // this handler.
          onLongPress={busy.isBusy(item.id) ? undefined : setMenuCoupon}
        />
      );
      // A scheduled or expired coupon gets NO gesture container at all.
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
    [actionsFor, currency, handlePress, busy.isBusy],
  );

  return (
    <Screen>
      <CollapsingHeader
        eyebrow="MARKETING"
        title="Coupons"
        // Nested route under More → Marketing. The chevron gets its OWN nav
        // row when expanded, so the eyebrow, title, chips and rows all share
        // gutter 20.
        onBack={() => router.back()}
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/marketing/coupons/new")}
            accessibilityLabel="New coupon"
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
      ) : isError && coupons.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load coupons"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.View
          testID="coupons-list-wrap"
          style={styles.listWrap}
          entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}
        >
          <Animated.FlatList
            testID="coupons-list"
            data={coupons}
            renderItem={renderItem}
            keyExtractor={(item) => (item as Coupon).id}
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
                title="No coupons yet"
                message={
                  filter !== "all"
                    ? "No coupons with this status."
                    : "Create a discount code to get started."
                }
              />
            }
          />
        </Animated.View>
      )}

      {/* The CODE, not the title: it is what the merchant recognises a coupon
          by, and `ActionSheet` clamps it to the one line its height budget
          reserves. */}
      <ActionSheet
        title={menuCoupon?.code}
        items={menuItems}
        visible={menuCoupon !== null}
        onDismiss={() => setMenuCoupon(null)}
      />

      {/* Why the last swipe changed nothing. Floats above the dock and
          replaces itself rather than stacking. */}
      <ActionFailureNotice failure={busy.failure} onDismiss={busy.dismissFailure} />
    </Screen>
  );
}

const styles = StyleSheet.create({
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

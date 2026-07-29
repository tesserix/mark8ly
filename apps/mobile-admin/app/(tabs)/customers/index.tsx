import { useState, useCallback, useEffect, useMemo, useRef } from "react";
import {
  Alert,
  View,
  Linking,
  Pressable,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { Star } from "lucide-react-native";
import Animated, { FadeIn, useReducedMotion } from "react-native-reanimated";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { useCustomers } from "../../../lib/hooks/use-customers";
import { useBlockCustomer, useUnblockCustomer } from "../../../lib/admin-api/customer-actions";
import { CustomerRow } from "../../../components/CustomerRow";
import {
  ActionFailureNotice,
  ActionSheet,
  CollapsingHeader,
  EmptyState,
  FilterChips,
  Screen,
  SearchField,
  Text,
  type ActionSheetItem,
} from "@/components/ui";
import {
  BlockReasonSheet,
  type BlockReasonSheetHandle,
} from "@/components/customers/BlockReasonSheet";
import { useBusyIds } from "@/lib/use-busy-ids";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
import { customerIdentity } from "@/lib/customer-identity";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING } from "@/components/products/disclosure-motion";
import type { Customer } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "active" | "blocked";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  { key: "blocked", label: "Blocked" },
];

const BLOCK_ERROR = "Couldn't block this customer. Try again.";

function useDebounce(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

/**
 * Everything a dialler will not accept, stripped.
 *
 * A merchant's address book format is not the app's to assume: a phone
 * arrives as `(02) 9999 8888`, `+61 400 123 456` or `02-9999-8888`, and
 * `tel:(02) 9999 8888` is not a URL any dialler parses — the spaces alone
 * make it invalid. Digits and a leading `+` survive; everything else goes.
 */
function telUrl(phone: string): string {
  return `tel:${phone.replace(/[^\d+]/g, "")}`;
}

/**
 * Hands a URL to the OS, and says so if nothing can open it.
 *
 * `Linking.openURL` REJECTS when no app can handle the scheme — an iPad or a
 * simulator with no dialler, a device with no mail account. Left bare that is
 * a tap that does nothing at all, with no explanation; the rejection is
 * caught and turned into the one thing the merchant can act on.
 *
 * `Promise.resolve(...)` wraps it because `openURL` is only a promise on a
 * real platform — the same defensive shape `refresh()` uses on Orders.
 */
function openExternal(url: string, failure: string): void {
  void Promise.resolve(Linking.openURL(url)).catch(() => {
    Alert.alert("Couldn't open", failure);
  });
}

/**
 * Customers — the address book, worked with a thumb.
 *
 * Two affordances, not the three Orders and Products have:
 *
 *  1. Long press → the four-action menu (Email, Call, Block, Unblock), always
 *     four items so the sheet never resizes under the thumb.
 *  2. Tap → the customer detail.
 *
 * There is deliberately NO SWIPE. The one state-changing action here is
 * Block, and `BlockCustomerRequest.Reason` is `binding:"required"` — an
 * action that cannot complete without typed input can never be a
 * fire-on-tap gesture, and a revealed button that only ever opens a sheet
 * teaches the wrong thing about the row. Unblock is direct and idempotent
 * but has no constructive pair worth arming a whole gesture for. The absence
 * is asserted in `__tests__/customers-screen.test.tsx`, so a later screen
 * adding one has to justify it.
 *
 * Search stays PINNED above the chips — the scroll-revealed variant was
 * proven undiscoverable on device and reverted on Orders.
 *
 * NO OPTIMISTIC HIDE and no optimistic update. `useBlockCustomer` and
 * `useUnblockCustomer` both invalidate `["customers"]`, which prefix-matches
 * this screen's own `["customers", search, status]` key, so the refetch is
 * authoritative about this list's contents: under the Blocked chip the
 * actioned customer drops in or out by itself, and under All their badge
 * simply flips. What the absent hide leaves open — a second action on a row
 * whose request is still in flight — is closed by suppressing THAT row's
 * long-press (`useBusyIds`), a guard on the control keyed purely on the
 * mutation lifecycle.
 */
export default function CustomersScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const reduceMotion = useReducedMotion();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);
  const currencyCode = useTenantStore((s) => s.activeStore?.currency_code);

  // Scroll offset the CollapsingHeader reads. Straight through, no rebase:
  // search is pinned above the list, so the list starts at 0 and the header's
  // resting state and the list's resting position already agree.
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
  } = useCustomers({
    ...(activeFilter !== "all" ? { status: activeFilter } : {}),
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
  });

  const customers = useMemo(
    () => data?.pages.flatMap((page) => page.data) ?? [],
    [data],
  );

  const busy = useBusyIds();
  const blockCustomer = useBlockCustomer();
  const unblockCustomer = useUnblockCustomer();

  // The customer whose long-press menu is open. Also the only thing keeping
  // the menu mounted — `ActionSheet` is a controlled component.
  const [menuCustomer, setMenuCustomer] = useState<Customer | null>(null);
  // The customer the reason sheet is about. Released on EVERY close (see the
  // sheet's `onDismiss` below), never only on a successful block.
  const [blockTarget, setBlockTarget] = useState<Customer | null>(null);
  // Local, NOT `blockCustomer.error`. react-query never resets a mutation
  // error, so binding the sheet straight to it means one failed block greets
  // the merchant on EVERY subsequent customer's sheet before they type
  // anything. Cleared on present.
  const [blockError, setBlockError] = useState<string | null>(null);
  const blockSheetRef = useRef<BlockReasonSheetHandle>(null);

  const handlePress = useCallback(
    (customer: Customer) => router.push(`/(tabs)/customers/${customer.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const openBlockSheet = useCallback((customer: Customer) => {
    setBlockTarget(customer);
    setBlockError(null);
    blockSheetRef.current?.present();
  }, []);

  const submitBlock = useCallback(
    (reason: string) => {
      const id = blockTarget?.id;
      // Nothing armed — the merchant backed out of this sheet. The guard is
      // reachable because the sheet is always mounted (portal), so it is a
      // real gate, not belt-and-braces.
      if (!id) return;
      blockCustomer.mutate(
        { id, reason },
        {
          onSuccess: () => {
            blockSheetRef.current?.dismiss();
            setBlockTarget(null);
            void adminHaptics.actionSucceeded();
          },
          onError: () => {
            // The sheet stays OPEN, holding the typed reason, so a retry
            // costs no retyping.
            setBlockError(BLOCK_ERROR);
            void adminHaptics.actionFailed();
          },
        },
      );
    },
    [blockTarget, blockCustomer],
  );

  const unblock = useCallback(
    (customer: Customer) => {
      busy.markBusy(customer.id);
      unblockCustomer.mutate(
        customer.id,
        // The action label matters more here than anywhere else in the app:
        // `CustomerRow` carries no status badge, so under the default "All"
        // filter a failed unblock and a successful one render IDENTICALLY.
        // Without a message the only difference is a haptic.
        busy.settleCallbacks(customer.id, "unblock this customer"),
      );
    },
    // The two STABLE callbacks off `busy`, not the whole object: its identity
    // changes on every busy transition, so depending on it would re-derive
    // this callback — and therefore every row's actions — each time a
    // mutation starts or settles.
    [unblockCustomer, busy.markBusy, busy.settleCallbacks],
  );

  /**
   * The long-press menu. ALWAYS these four items, in this order — illegal
   * ones are `disabled` rather than dropped, because `ActionSheet`'s
   * `snapPoints` memoises on `items.length` and a dropped item resizes the
   * sheet under the merchant's thumb.
   *
   * Unblock is here even though §3 of the spec lists only three items: Block
   * is only reversible from a phone if it is, and the same argument that
   * forced the Archived chip onto Products applies unchanged.
   */
  const menuItems = useMemo((): ActionSheetItem[] => {
    const target = menuCustomer;
    if (!target) return [];
    const isBlocked = target.status === "blocked";
    return [
      {
        key: "email",
        label: "Email",
        // Never disabled: `email` is the one non-optional identity field on
        // the wire (customers_dto.go), so every customer has one.
        onPress: () =>
          openExternal(`mailto:${target.email.trim()}`, "No mail app is set up on this device."),
      },
      {
        key: "call",
        label: "Call",
        // Gated on the PHONE, not on the name — an imported customer routinely
        // has one and not the other.
        disabled: !target.phone,
        onPress: () =>
          target.phone
            ? openExternal(telUrl(target.phone), "No dialler is available on this device.")
            : undefined,
      },
      {
        key: "block",
        label: "Block",
        tone: "danger",
        disabled: isBlocked,
        // Opens the sheet and sends NOTHING. The reason is required
        // server-side, so a bare tap could only ever produce a 400.
        onPress: () => openBlockSheet(target),
      },
      {
        key: "unblock",
        label: "Unblock",
        disabled: !isBlocked,
        onPress: () => unblock(target),
      },
    ];
  }, [menuCustomer, openBlockSheet, unblock]);

  const renderItem = useCallback(
    ({ item }: { item: Customer }) => (
      <CustomerRow
        customer={item}
        onPress={handlePress}
        // Suppressed while THIS row's own request is open, so a still-visible
        // row can't be fired at twice. Not a claim about the data — see the
        // screen's doc comment.
        onLongPress={busy.isBusy(item.id) ? undefined : setMenuCustomer}
        currencyCode={currencyCode}
      />
    ),
    [handlePress, currencyCode, busy.isBusy],
  );

  return (
    <Screen>
      <CollapsingHeader
        eyebrow="PEOPLE"
        title="Customers"
        rightSlot={
          <Pressable
            style={styles.reviewsLink}
            onPress={() => router.push("/(tabs)/customers/reviews")}
            accessibilityRole="button"
            accessibilityLabel="View customer reviews"
          >
            <Star size={16} color={theme.colors.text} strokeWidth={1.75} />
            {/* The slot's OWN contract: `CollapsingHeader` sizes its box for
                one line of a slot it doesn't otherwise own. */}
            <Text preset="caption" color="text" numberOfLines={1}>
              Reviews
            </Text>
          </Pressable>
        }
        scrollY={scrollY}
      />
      <View style={styles.search}>
        <SearchField
          value={searchText}
          onChangeText={setSearchText}
          placeholder="Search customers…"
          accessibilityLabel="Search customers"
        />
      </View>
      {/* Pills, matching Orders and Products. `all` sends no `status` at all;
          every other key IS the `status` value the backend takes. */}
      <FilterChips<FilterKey>
        chips={FILTERS}
        value={activeFilter}
        onChange={setActiveFilter}
      />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && customers.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load customers"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.View
          testID="customers-list-wrap"
          style={styles.listWrap}
          entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}
        >
          <Animated.FlatList
            testID="customers-list"
            data={customers}
            renderItem={renderItem}
            keyExtractor={(item) => (item as Customer).id}
            onScroll={scrollHandler}
            scrollEventThrottle={16}
            // No `ListHeaderComponent` and no `contentOffset`: search is
            // pinned above the list, so the list shows rows from its first
            // pixel.
            style={styles.listFlex}
            contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
            keyboardShouldPersistTaps="handled"
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
                title="No customers yet"
                message={
                  debouncedSearch || activeFilter !== "all"
                    ? "Try a different search or filter."
                    : "Customers appear here once they sign up."
                }
              />
            }
          />
        </Animated.View>
      )}

      {/* ONE source for "who is this", shared with the row and the detail
          screen — see lib/customer-identity.ts. For a customer with no name
          the title IS their email, and there is no subtitle to duplicate it
          with. */}
      <ActionSheet
        title={menuCustomer ? customerIdentity(menuCustomer).title : undefined}
        items={menuItems}
        visible={menuCustomer !== null}
        onDismiss={() => setMenuCustomer(null)}
      />

      {/* Why the last action changed nothing — the one signal this screen
          has, since its rows carry no status badge to flip. Floats above the
          dock and replaces itself rather than stacking. */}
      <ActionFailureNotice failure={busy.failure} onDismiss={busy.dismissFailure} />

      <BlockReasonSheet
        ref={blockSheetRef}
        customerLabel={blockTarget ? customerIdentity(blockTarget).title : undefined}
        onSubmit={submitBlock}
        isSubmitting={blockCustomer.isPending}
        error={blockError}
        // Released on EVERY close, not only on a successful block. This sheet
        // is always mounted, so a target left pinned by a backed-out sheet is
        // a live, armed submit for a customer the merchant walked away from —
        // the exact bug Orders shipped with its cancel sheet.
        onDismiss={() => setBlockTarget(null)}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  search: {
    // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so
    // the search field aligns with the rows below it. Not theme.spacing.lg.
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xs,
  },
  reviewsLink: {
    flexDirection: "row",
    alignItems: "center",
    gap: 4,
    minHeight: 44,
  },
  listWrap: {
    flex: 1,
  },
  listFlex: { flex: 1 },
  list: {
    flexGrow: 1,
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

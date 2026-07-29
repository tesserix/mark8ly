import { useState, useCallback, useEffect, useMemo } from "react";
import { View, Alert, RefreshControl, StyleSheet, ActivityIndicator } from "react-native";
import { useRouter } from "expo-router";
import { Check, FileText, Plus } from "lucide-react-native";
import Animated, { FadeIn, useReducedMotion } from "react-native-reanimated";
import { useProducts } from "@/lib/hooks/use-products";
import { useSetProductStatus } from "@/lib/admin-api/product-status";
import { ProductRow } from "@/components/ProductRow";
import {
  ActionSheet,
  CollapsingHeader,
  EmptyState,
  FilterChips,
  Hairline,
  IconButton,
  MAX_FONT_SCALE,
  Screen,
  SearchField,
  SwipeRow,
  Text,
  type ActionSheetItem,
  type SwipeAction,
} from "@/components/ui";
import { useBusyIds } from "@/lib/use-busy-ids";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING } from "@/components/products/disclosure-motion";
import type { Product } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "active" | "draft" | "archived";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  // Was "Inactive" -> status=inactive, a hard 400: the backend enum is
  // draft|active|archived. 149 of the store's 161 products are drafts.
  { key: "draft", label: "Draft" },
  // NOT optional, and not cosmetic. The long-press menu can archive a
  // product; without this chip an archived product is absent from Active,
  // absent from Draft, and reachable only by scrolling the whole catalogue
  // under All — so Archive becomes effectively irreversible from a phone.
  // The key IS the status value the backend matches (`status = ?`,
  // repository.go:264, validated `oneof=draft active archived`).
  { key: "archived", label: "Archived" },
];

const ICON_SIZE = 20;

function useDebounce(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

/**
 * Products — the catalogue, worked with a thumb.
 *
 * Three affordances, in increasing order of commitment, exactly as Orders:
 *
 *  1. Swipe right → Activate (publish). Swipe left → Set to draft
 *     (unpublish). Per the app-wide convention, constructive at the leading
 *     edge in moss. The trailing action is `neutral`, NOT danger: unpublishing
 *     is dismissive, reversible and idempotent, and the trailing edge is a
 *     POSITION not a tone (the Dashboard's ticket Close row proves the same
 *     point). Neither fires on the drag — `SwipeRow` settles open and the
 *     revealed button is tapped; nothing in this app opts into
 *     `autoFireOnFullSwipe` because there is no undo.
 *  2. Long press → the four-action menu, always four items so the sheet never
 *     resizes under the thumb (see `menuItems`).
 *  3. Tap → the product detail, where everything is editable.
 *
 * Search stays PINNED above the chips. The scroll-revealed variant was
 * proven undiscoverable on device and reverted on Orders; do not reintroduce
 * it here.
 *
 * NO OPTIMISTIC HIDE, and no optimistic update. `useSetProductStatus`
 * invalidates ["products"], which prefix-matches this screen's own
 * ["products", status, search] key, so the refetch is authoritative about
 * this list's contents: under a status filter the actioned product drops out
 * by itself, and under All its badge simply flips. The Dashboard needed a
 * local overlay only because its queue is built from a DIFFERENT query that
 * its own row actions did not invalidate; that asymmetry does not exist here,
 * and re-creating the dismissed/watermark machinery would import a
 * four-round bug for no gain.
 *
 * What the absent hide leaves open — a second action on a row whose request
 * is still in flight — is closed by disabling THAT row's gesture AND its
 * long-press (`useBusyIds`). A guard on the control, keyed purely on the
 * mutation lifecycle: "the request stopped" re-arms a button and is never
 * evidence that fresh data arrived.
 */
export default function ProductsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const reduceMotion = useReducedMotion();
  const [activeFilter, setActiveFilter] = useState<FilterKey>("all");
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);

  // Scroll offset the CollapsingHeader reads. Straight through, no rebase:
  // search is pinned above the list, so the list starts at 0 and the header's
  // resting state and the list's resting position already agree.
  const { scrollY, onScroll: scrollHandler } = useCollapsingScroll();

  const queryParams = {
    ...(activeFilter !== "all" ? { status: activeFilter } : {}),
    ...(debouncedSearch ? { search: debouncedSearch } : {}),
  };

  const { data, isLoading, isRefetching, isError, refetch } = useProducts(
    Object.keys(queryParams).length > 0 ? queryParams : undefined,
  );

  const products = useMemo(() => data?.data ?? [], [data]);
  /**
   * The header count, read off the VISIBLE query's own meta.
   *
   * Deliberately unlike Orders, which pins its count to a separate
   * `status: "pending"` query. That count describes a scope the merchant may
   * not be looking at, so deriving it from a filtered list would report
   * "3 pending" on the Cancelled tab. This one describes the scope on screen,
   * so the visible query is not merely the cheap source — it is the only one
   * that can stay true as the chips change, and it costs no extra request.
   */
  const total = data?.meta?.total ?? 0;

  const busy = useBusyIds();
  const setStatus = useSetProductStatus();
  // The product whose long-press menu is open. Also the only thing keeping
  // the menu mounted — `ActionSheet` is a controlled component.
  const [menuProduct, setMenuProduct] = useState<Product | null>(null);

  const handlePress = useCallback(
    (product: Product) => router.push(`/(tabs)/products/${product.id}`),
    [router],
  );

  const setProductStatus = useCallback(
    (product: Product, status: "draft" | "active" | "archived") => {
      busy.markBusy(product.id);
      setStatus.mutate({ id: product.id, status }, busy.settleCallbacks(product.id));
    },
    // The two STABLE callbacks off `busy`, not the whole object: its identity
    // changes on every busy transition, so depending on it would re-derive
    // this callback — and therefore every row's actions — each time a
    // mutation starts or settles.
    [setStatus, busy.markBusy, busy.settleCallbacks],
  );

  /**
   * Archive is the one action here a merchant cannot walk back with the same
   * thumb motion, so it is the one behind a confirm. The copy names the chip
   * that reverses it — which is why that chip is not optional (see FILTERS).
   */
  const confirmArchive = useCallback(
    (product: Product) => {
      Alert.alert(
        "Archive product?",
        `"${product.title}" will be removed from your storefront. You can find it again under the Archived filter and re-activate it.`,
        [
          // No `onPress`: cancelling must be inert, not a no-op handler that
          // could later grow a body.
          { text: "Cancel", style: "cancel" },
          {
            text: "Archive",
            style: "destructive",
            onPress: () => setProductStatus(product, "archived"),
          },
        ],
      );
    },
    [setProductStatus],
  );

  /**
   * Swipe actions for one product, gated on what its status makes meaningful.
   *
   * `PATCH /products/:id {status}` is idempotent with no transition rules
   * behind it, so — unlike Orders — nothing here would 4xx. The gates exist
   * for the merchant, not the API: an armed gesture that can only no-op is
   * worse than no gesture, and it teaches the wrong thing about a row.
   *
   * An ARCHIVED product therefore keeps its leading Activate, and that is
   * deliberate rather than an oversight: re-activating is the whole point of
   * the Archived chip, and a one-thumb motion is the most direct route back.
   * (The task brief's example carried a comment claiming an archived row gets
   * neither gesture, contradicting its own code one line above; the code is
   * the version that makes Archive reversible, so the code wins and the
   * unreachable `return null` branch it implied is not carried over. Every
   * row here has at least one legal action, so unlike Orders — where a
   * terminal order genuinely has none — there is no "no SwipeRow" case to
   * guard, and an unreachable guard no test can exercise is just
   * unverifiable code.)
   */
  const actionsFor = useCallback(
    (product: Product): { leading: SwipeAction[]; trailing: SwipeAction[] } => {
      const leading: SwipeAction[] =
        product.status === "active"
          ? []
          : [
              {
                key: "activate",
                label: "Activate",
                tone: "accent",
                icon: <Check size={ICON_SIZE} color={theme.colors.inverse} strokeWidth={2} />,
                onPress: () => setProductStatus(product, "active"),
              },
            ];

      const trailing: SwipeAction[] =
        product.status === "active"
          ? [
              {
                key: "draft",
                label: "Draft",
                // Dismissive, not destructive — see the screen doc comment.
                tone: "neutral",
                icon: <FileText size={ICON_SIZE} color={theme.colors.text} strokeWidth={2} />,
                onPress: () => setProductStatus(product, "draft"),
              },
            ]
          : [];

      return { leading, trailing };
    },
    [setProductStatus],
  );

  /**
   * The long-press menu. ALWAYS these four items, in this order — illegal
   * ones are `disabled` rather than dropped, because `ActionSheet`'s
   * `snapPoints` memoises on `items.length` and a dropped item resizes the
   * sheet under the merchant's thumb. See `ActionSheetItem.disabled`.
   */
  const menuItems = useMemo((): ActionSheetItem[] => {
    const target = menuProduct;
    if (!target) return [];
    return [
      {
        key: "edit",
        label: "Edit",
        onPress: () => handlePress(target),
      },
      {
        key: "activate",
        label: "Activate",
        disabled: target.status === "active",
        onPress: () => setProductStatus(target, "active"),
      },
      {
        key: "draft",
        label: "Set to draft",
        disabled: target.status === "draft",
        onPress: () => setProductStatus(target, "draft"),
      },
      {
        key: "archive",
        label: "Archive",
        tone: "danger",
        disabled: target.status === "archived",
        onPress: () => confirmArchive(target),
      },
    ];
  }, [menuProduct, handlePress, setProductStatus, confirmArchive]);

  const renderItem = useCallback(
    ({ item, index }: { item: Product; index: number }) => {
      const actions = actionsFor(item);
      const row = (
        <ProductRow
          product={item}
          onPress={handlePress}
          // Gated on the SAME busy set as the swipe below: the menu is a
          // second route onto the row, and Archive is not idempotent in the
          // way a merchant experiences it.
          onLongPress={busy.isBusy(item.id) ? undefined : setMenuProduct}
        />
      );
      return (
        <View>
          {index > 0 ? <Hairline inset={theme.spacing.xl} /> : null}
          <SwipeRow
            testID={`swipe-${item.id}`}
            leadingActions={actions.leading}
            trailingActions={actions.trailing}
            // Suppressed while THIS row's own request is open, so a
            // still-visible row can't be fired at twice. Not a claim about
            // the data — see the screen's doc comment.
            enabled={!busy.isBusy(item.id)}
          >
            {row}
          </SwipeRow>
        </View>
      );
    },
    [actionsFor, handlePress, busy.isBusy],
  );

  return (
    <Screen>
      <CollapsingHeader
        eyebrow="CATALOGUE"
        title="Products"
        rightSlot={
          total > 0 ? (
            <Text
              preset="caption"
              color="textTertiary"
              style={styles.count}
              numberOfLines={1}
              // Explicit, not inherited: `CollapsingHeader` computes its own
              // height from this exact multiplier, so a slot it doesn't own
              // has to state that it honours the same contract.
              maxFontSizeMultiplier={MAX_FONT_SCALE}
              testID="products-count"
            >
              {total} {total === 1 ? "product" : "products"}
            </Text>
          ) : undefined
        }
        scrollY={scrollY}
      />

      <View style={styles.search}>
        <SearchField
          value={searchText}
          onChangeText={setSearchText}
          placeholder="Search products…"
          accessibilityLabel="Search products"
        />
      </View>
      {/* Pills, not the old underlined tabs — the same strip Orders uses, in
          the same place relative to the pinned search field. Semantics are
          untouched: the key IS the `status` query value (see FILTERS). */}
      <FilterChips<FilterKey>
        chips={FILTERS}
        value={activeFilter}
        onChange={setActiveFilter}
      />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && products.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load products"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.View
          testID="products-list-wrap"
          style={styles.listWrap}
          entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}
        >
          <Animated.FlatList
            testID="products-list"
            data={products}
            renderItem={renderItem}
            keyExtractor={(item) => (item as Product).id}
            onScroll={scrollHandler}
            scrollEventThrottle={16}
            // No `ListHeaderComponent` and no `contentOffset`: search is
            // pinned above the list, so the list shows rows from its first
            // pixel.
            style={styles.listFlex}
            contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
            keyboardShouldPersistTaps="handled"
            refreshControl={
              <RefreshControl
                refreshing={isRefetching}
                onRefresh={refetch}
                tintColor={theme.colors.text}
              />
            }
            // Three genuinely different empty moments, and the screen used
            // to tell a merchant with 162 products to "add your first
            // product" whenever a FILTER came back empty. The Archived chip
            // is what made that visible — it is routinely empty — but the
            // Active and Draft chips had the same lie in them all along.
            ListEmptyComponent={
              <EmptyState
                align="left"
                title={
                  debouncedSearch || activeFilter !== "all"
                    ? "Nothing here"
                    : "No products yet"
                }
                message={
                  debouncedSearch
                    ? "Try a different search term."
                    : activeFilter !== "all"
                      ? `No ${activeFilter} products right now.`
                      : "Add your first product to get started."
                }
              />
            }
          />
        </Animated.View>
      )}

      <ActionSheet
        title={menuProduct?.title}
        items={menuItems}
        visible={menuProduct !== null}
        onDismiss={() => setMenuProduct(null)}
      />

      <IconButton
        onPress={() => router.push("/(tabs)/products/new")}
        accessibilityLabel="Add new product"
        tone="onDark"
        style={[styles.fab, { bottom: dockPad }]}
      >
        <Plus size={22} color={theme.colors.inverse} strokeWidth={2} />
      </IconButton>
    </Screen>
  );
}

const styles = StyleSheet.create({
  search: {
    // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so
    // the search field aligns with ProductRow below it. Not theme.spacing.lg.
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.xs,
  },
  listWrap: {
    flex: 1,
  },
  listFlex: { flex: 1 },
  list: {
    flexGrow: 1,
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
  count: { fontVariant: ["tabular-nums"] },
  // `bottom` is applied at the call site from useDockClearance() — the dock
  // is absolutely positioned and renders above this, so a static bottom
  // (it was theme.spacing.lg) parked the FAB underneath it and Add-product
  // was unreachable.
  fab: {
    position: "absolute",
    right: theme.spacing.lg,
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: theme.colors.accent,
    alignItems: "center",
    justifyContent: "center",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.18,
    shadowRadius: 10,
    elevation: 6,
  },
});

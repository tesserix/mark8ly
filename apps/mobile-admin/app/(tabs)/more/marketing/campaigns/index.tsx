import { useCallback, useMemo, useState } from "react";
import {
  View,
  Alert,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import Animated from "react-native-reanimated";
import { useRouter } from "expo-router";
import { Plus } from "lucide-react-native";
import { useCampaigns } from "@/lib/hooks/use-campaigns";
import { useDeleteCampaign } from "@/lib/admin-api/campaign-actions";
import { CampaignRow } from "@/components/marketing/CampaignRow";
import {
  ActionFailureNotice,
  ActionSheet,
  CollapsingHeader,
  EmptyState,
  FilterChips,
  IconButton,
  Screen,
  type ActionSheetItem,
} from "@/components/ui";
import { useBusyIds } from "@/lib/use-busy-ids";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
import { theme } from "@/lib/theme";
import type { Campaign } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "draft" | "scheduled" | "sent" | "paused";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "draft", label: "Draft" },
  { key: "scheduled", label: "Scheduled" },
  { key: "sent", label: "Sent" },
  { key: "paused", label: "Paused" },
];

/**
 * The one status a campaign may be deleted in.
 *
 * `DELETE /campaigns/:id` refuses anything else with a 409
 * `campaign_not_draft` (campaign/service.go:197-205), and a second call on a
 * campaign that DID delete comes back 404. Named here rather than inlined so
 * the menu's gate and the reason it exists sit together.
 */
const DELETABLE_STATUS = "draft";

/**
 * Campaigns — a list a merchant mostly READS, with exactly one destructive
 * action hidden behind two deliberate steps.
 *
 * Two affordances, not the three Orders and Products have:
 *
 *  1. Long press → the two-item menu (Edit, Delete). Always two items, so the
 *     sheet never resizes under the thumb; Delete is `disabled` on anything
 *     that is not a draft rather than dropped.
 *  2. Tap → the campaign detail, where everything editable lives.
 *
 * There is deliberately NO SWIPE, and it is the strongest case in the app for
 * that absence. The only state-changing action reachable from this list is a
 * permanent delete: it is ILLEGAL on most rows (a sent or scheduled campaign
 * is a guaranteed 409) and UNRECOVERABLE on the rest, in an app that has no
 * undo. A gesture that mostly cannot fire, and destroys a record when it can,
 * is the worst possible fit for a thumb. The absence is asserted in
 * `__tests__/campaigns-screen.test.tsx`, so a later screen adding one has to
 * justify it.
 *
 * Duplicate is CUT rather than deferred: the admin API has no duplicate
 * endpoint, and a menu item that can only 404 is worse than no item at all.
 *
 * NO OPTIMISTIC HIDE, and no optimistic update. `useDeleteCampaign`
 * invalidates ["campaigns"], which prefix-matches this screen's own
 * ["campaigns", "list", status] key, so the refetch is authoritative about
 * this list's contents — the deleted row leaves by itself, and a REFUSED
 * delete leaves the row exactly where it was rather than blinking out and
 * back. What the absent hide leaves open — a second action on a row whose
 * request is still in flight — is closed by suppressing THAT row's long-press
 * (`useBusyIds`), a guard on the control keyed purely on the mutation
 * lifecycle.
 */
export default function CampaignsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const [filter, setFilter] = useState<FilterKey>("all");

  // Scroll offset the CollapsingHeader reads. Straight through, no rebase:
  // the chips are pinned above the list, so the list starts at 0 and the
  // header's resting state and the list's resting position already agree.
  const { scrollY, onScroll } = useCollapsingScroll();

  const {
    data,
    isLoading,
    isRefetching,
    isError,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useCampaigns(filter !== "all" ? { status: filter } : undefined);

  const campaigns = useMemo(
    () => data?.pages.flatMap((page) => page.data) ?? [],
    [data],
  );

  const busy = useBusyIds();
  const deleteCampaign = useDeleteCampaign();
  // The campaign whose long-press menu is open. Also the only thing keeping
  // the menu mounted — `ActionSheet` is a controlled component.
  const [menuCampaign, setMenuCampaign] = useState<Campaign | null>(null);

  const handlePress = useCallback(
    (campaign: Campaign) => router.push(`/(tabs)/more/marketing/campaigns/${campaign.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const removeCampaign = useCallback(
    (campaign: Campaign) => {
      busy.markBusy(campaign.id);
      deleteCampaign.mutate(
        campaign.id,
        // The action label is what turns a bare failure haptic into
        // "Couldn't delete this campaign — <the server's reason>". Without it
        // a refusal is indistinguishable from a delete still in flight: no
        // optimistic hide runs, so the row looks identical either way.
        busy.settleCallbacks(campaign.id, "delete this campaign"),
      );
    },
    // The two STABLE callbacks off `busy`, not the whole object: its identity
    // changes on every busy transition, so depending on it would re-derive
    // this callback — and therefore every row's actions — each time a
    // mutation starts or settles.
    [deleteCampaign, busy.markBusy, busy.settleCallbacks],
  );

  /**
   * The confirm. Delete is permanent and there is no undo anywhere in this
   * app, so the campaign is NAMED — a merchant who long-pressed the wrong row
   * finds out here rather than afterwards.
   */
  const confirmDelete = useCallback(
    (campaign: Campaign) => {
      Alert.alert(
        "Delete campaign?",
        `"${campaign.name}" will be permanently deleted. This cannot be undone.`,
        [
          // No `onPress`: cancelling must be inert, not a no-op handler that
          // could later grow a body.
          { text: "Cancel", style: "cancel" },
          {
            text: "Delete",
            style: "destructive",
            onPress: () => removeCampaign(campaign),
          },
        ],
      );
    },
    [removeCampaign],
  );

  /**
   * The long-press menu. ALWAYS these two items, in this order — an illegal
   * Delete is `disabled` rather than dropped, because `ActionSheet`'s
   * `snapPoints` memoises on `items.length` and a dropped item resizes the
   * sheet under the merchant's thumb. See `ActionSheetItem.disabled`.
   *
   * The gate is `status !== "draft"`, matching the server exactly. It is a
   * courtesy, not a safety net: a campaign sent from the web after this
   * list's last refetch still reaches the API and still comes back 409 — see
   * the failure notice below, which is what tells the merchant so.
   */
  const menuItems = useMemo((): ActionSheetItem[] => {
    const target = menuCampaign;
    if (!target) return [];
    return [
      {
        key: "edit",
        label: "Edit",
        onPress: () => handlePress(target),
      },
      {
        key: "delete",
        label: "Delete",
        tone: "danger",
        disabled: target.status !== DELETABLE_STATUS,
        onPress: () => confirmDelete(target),
      },
    ];
  }, [menuCampaign, handlePress, confirmDelete]);

  const renderItem = useCallback(
    ({ item }: { item: Campaign }) => (
      <CampaignRow
        campaign={item}
        onPress={handlePress}
        // Suppressed while THIS row's own request is open, so a still-visible
        // row can't be fired at twice — and a delete is exactly the action
        // where a double fire produces a 404 on the second call.
        onLongPress={busy.isBusy(item.id) ? undefined : setMenuCampaign}
      />
    ),
    [handlePress, busy.isBusy],
  );

  return (
    <Screen>
      <CollapsingHeader
        eyebrow="MARKETING"
        title="Campaigns"
        // This is a NESTED route — it used `BackHeader`, which had a chevron
        // this primitive did not. `onBack` is the additive prop that closed
        // that gap; without it the merchant would be stranded here. It also
        // puts the chevron in its OWN nav row while expanded, so the eyebrow,
        // title, chips and rows all start at gutter 20.
        onBack={() => router.back()}
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/marketing/campaigns/new")}
            accessibilityLabel="New campaign"
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </IconButton>
        }
        scrollY={scrollY}
      />
      {/* Pills, matching Orders. Pinned OUTSIDE the list and above it — not in
          `ListHeaderComponent`: `FilterChips` owns its own vertical rhythm and
          a hugging wrapper, and inside the list that wrapper stretches and
          leaves ~110pt of dead paper between the header and the first row.
          Semantics untouched — `all` sends no `status`, every other key IS
          the `status` value. */}
      <FilterChips<FilterKey> chips={FILTERS} value={filter} onChange={setFilter} />

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && campaigns.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load campaigns"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.FlatList
          testID="campaigns-list"
          data={campaigns}
          renderItem={renderItem}
          keyExtractor={(item) => (item as Campaign).id}
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
              title="No campaigns yet"
              message={
                filter !== "all"
                  ? "No campaigns with this status."
                  : "Create an email campaign to reach your customers."
              }
            />
          }
        />
      )}

      <ActionSheet
        title={menuCampaign?.name}
        items={menuItems}
        visible={menuCampaign !== null}
        onDismiss={() => setMenuCampaign(null)}
      />

      {/* Why the last delete changed nothing. The row is never hidden
          optimistically, so a refusal and a delete still in flight render
          IDENTICALLY — this strip is the only difference between them. It
          floats above the dock and replaces itself rather than stacking. */}
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

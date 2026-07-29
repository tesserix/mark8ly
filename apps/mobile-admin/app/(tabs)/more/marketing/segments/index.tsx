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
import { useSegments } from "@/lib/hooks/use-segments";
import { useDeleteSegment } from "@/lib/admin-api/segment-actions";
import { SegmentRow } from "@/components/marketing/SegmentRow";
import {
  ActionFailureNotice,
  ActionSheet,
  CollapsingHeader,
  EmptyState,
  IconButton,
  Screen,
  type ActionSheetItem,
} from "@/components/ui";
import { useBusyIds } from "@/lib/use-busy-ids";
import { useCollapsingScroll } from "@/lib/use-collapsing-scroll";
import { theme } from "@/lib/theme";
import type { Segment } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

/**
 * Segments — the audiences campaigns are aimed at.
 *
 * Two affordances:
 *
 *  1. Long press → the two-item menu (Edit, Delete). Always two items so the
 *     sheet never resizes under the thumb.
 *  2. Tap → the segment detail, where the rules are edited.
 *
 * NO SWIPE. The one state-changing action here is a HARD delete with no undo
 * — the least appropriate thing in the app to put behind a one-thumb gesture
 * — and Edit is navigation, which a tap already does. Asserted in
 * `__tests__/segments-screen.test.tsx` rather than left implicit.
 *
 * NO FILTER CHIPS either, and this is the only list screen in the app without
 * them. A segment has no status, no lifecycle and no axis to filter on
 * (`segmentSchema`: name, description, rules, member_count), so a strip here
 * could hold nothing but a lone "All" pill — 40pt of chrome that filters
 * nothing. Do not add one.
 *
 * DELETE IS NOT GATED CLIENT-SIDE, deliberately. `campaigns.segment_id` is a
 * plain FK and the service refuses to delete a segment any campaign still
 * points at — 409 `segment_in_use` from the pre-check
 * (campaign/service.go:273-286), and again from the Postgres FK translation
 * for the TOCTOU window (campaign/repository.go:125-141). NOTHING on this
 * list says which segments those are: the wire carries no campaign-linkage
 * field, so a greyed-out Delete could only ever be a guess. The server holds
 * the fact and its refusal names the blocking campaign COUNT, so the item
 * stays enabled and the answer comes from the one place that has it. That
 * message reaches the merchant VERBATIM through `describeActionFailure` —
 * paraphrasing it would throw away the only actionable part, and the count
 * cannot be re-derived on the client.
 *
 * NO OPTIMISTIC HIDE. `useDeleteSegment` invalidates ["segments"] AND
 * ["campaigns"] (a campaign's audience is a fact about a segment that just
 * stopped existing), so the refetch is authoritative about this list's
 * contents. A refused delete therefore leaves the row exactly where it was,
 * which is correct: the merchant's next move is to retarget the campaign, not
 * to hunt for a row that blinked out and back.
 *
 * Pagination is unchanged: `GET /segments` returns a bare `{data}` with no
 * meta (segments.go:39), so this stays a plain query with no infinite scroll.
 */
export default function SegmentsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();

  // Scroll offset the CollapsingHeader reads. Straight through, no rebase:
  // the list is the header's immediate sibling and starts at 0.
  const { scrollY, onScroll } = useCollapsingScroll();

  const { data, isLoading, isRefetching, isError, refetch } = useSegments();
  const segments = useMemo(() => data?.data ?? [], [data]);

  const busy = useBusyIds();
  const deleteSegment = useDeleteSegment();
  // The segment whose long-press menu is open. Also the only thing keeping
  // the menu mounted — `ActionSheet` is a controlled component.
  const [menuSegment, setMenuSegment] = useState<Segment | null>(null);

  const handlePress = useCallback(
    (segment: Segment) => router.push(`/(tabs)/more/marketing/segments/${segment.id}`),
    [router],
  );

  const removeSegment = useCallback(
    (segment: Segment) => {
      busy.markBusy(segment.id);
      deleteSegment.mutate(
        segment.id,
        // The action label is what carries the server's refusal onto the
        // screen: "Couldn't delete this segment — segment is still used by 2
        // campaigns and cannot be deleted." Omit it and the only signal a
        // blocked delete produces is a haptic, against a row that looks the
        // same either way.
        busy.settleCallbacks(segment.id, "delete this segment"),
      );
    },
    // The two STABLE callbacks off `busy`, not the whole object: its identity
    // changes on every busy transition, so depending on it would re-derive
    // this callback — and therefore every row's actions — each time a
    // mutation starts or settles.
    [deleteSegment, busy.markBusy, busy.settleCallbacks],
  );

  /**
   * The confirm. It warns that the delete is permanent and NAMES the segment;
   * it does NOT try to predict whether the server will allow it. That answer
   * lives on the server (see the screen doc comment) and arrives afterwards,
   * in the failure notice — a confirm that guessed would be wrong about half
   * the time and would teach the merchant to distrust the one that isn't.
   */
  const confirmDelete = useCallback(
    (segment: Segment) => {
      Alert.alert(
        "Delete segment?",
        `"${segment.name}" will be permanently deleted. This cannot be undone.`,
        [
          // No `onPress`: cancelling must be inert, not a no-op handler that
          // could later grow a body.
          { text: "Cancel", style: "cancel" },
          {
            text: "Delete",
            style: "destructive",
            onPress: () => removeSegment(segment),
          },
        ],
      );
    },
    [removeSegment],
  );

  /**
   * The long-press menu. ALWAYS these two items, in this order —
   * `ActionSheet`'s `snapPoints` memoises on `items.length`, so a dropped
   * item would resize the sheet under the merchant's thumb. Neither is ever
   * conditional here, which is itself the decision: see the screen doc
   * comment on why Delete is not gated client-side.
   *
   * Duplicate is CUT: the admin API has no duplicate endpoint, and an item
   * that can only 404 is worse than no item at all.
   */
  const menuItems = useMemo((): ActionSheetItem[] => {
    const target = menuSegment;
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
        onPress: () => confirmDelete(target),
      },
    ];
  }, [menuSegment, handlePress, confirmDelete]);

  const renderItem = useCallback(
    ({ item }: { item: Segment }) => (
      <SegmentRow
        segment={item}
        onPress={handlePress}
        // Suppressed while THIS row's own request is open, so a still-visible
        // row can't be fired at twice — and a delete is exactly the action
        // where a double fire produces a 404 on the second call.
        onLongPress={busy.isBusy(item.id) ? undefined : setMenuSegment}
      />
    ),
    [handlePress, busy.isBusy],
  );

  return (
    <Screen>
      <CollapsingHeader
        eyebrow="MARKETING"
        title="Segments"
        // This is a NESTED route — it used `BackHeader`, which had a chevron
        // this primitive did not. `onBack` is the additive prop that closed
        // that gap; without it the merchant would be stranded here. It also
        // puts the chevron in its OWN nav row while expanded, so the eyebrow,
        // title and rows all start at gutter 20.
        onBack={() => router.back()}
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/marketing/segments/new")}
            accessibilityLabel="New segment"
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </IconButton>
        }
        scrollY={scrollY}
      />
      {/* No `FilterChips` — see the screen doc comment. The list is the
          header's immediate sibling here, which is why its scroll offset can
          drive `scrollY` with no rebase. */}

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && segments.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load segments"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <Animated.FlatList
          testID="segments-list"
          data={segments}
          renderItem={renderItem}
          keyExtractor={(item) => (item as Segment).id}
          // The pair that makes the header collapse at all: a plain FlatList
          // has nothing to feed `scrollY` and would freeze it expanded.
          onScroll={onScroll}
          scrollEventThrottle={16}
          style={styles.listFlex}
          contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
          // No `onEndReached`: `GET /segments` is unpaginated (bare `{data}`,
          // no meta), so there is no next page to ask for.
          refreshControl={
            <RefreshControl refreshing={isRefetching} onRefresh={refetch} tintColor={theme.colors.text} />
          }
          ListEmptyComponent={
            <EmptyState
              align="left"
              title="No segments yet"
              message="Group customers to target campaigns at the right audience."
            />
          }
        />
      )}

      <ActionSheet
        title={menuSegment?.name}
        items={menuItems}
        visible={menuSegment !== null}
        onDismiss={() => setMenuSegment(null)}
      />

      {/* Where a refused delete lands, carrying the server's own sentence —
          including the blocking campaign count, which exists nowhere else.
          The row is never hidden optimistically, so a refusal and a delete
          still in flight render IDENTICALLY; this strip is the only
          difference between them. It replaces itself rather than stacking. */}
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
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  // NOT `styles.centered` for the error state: that wrapper's
  // `alignItems: "center"` shrink-wraps and re-centres its child, silently
  // undoing `EmptyState align="left"`. Claims the remaining height only.
  errorSlot: { flex: 1 },
});

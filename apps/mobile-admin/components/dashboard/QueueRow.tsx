import { View, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { PressableRow, StatusBadge, Text, Thumb } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { QueueItem } from "@/lib/queue";

interface QueueRowProps {
  item: QueueItem;
  onPress: () => void;
}

function monogramInitial(label: string): string {
  const trimmed = label.trim();
  return trimmed ? trimmed.charAt(0).toUpperCase() : "?";
}

/**
 * The customer monogram tile — the fallback for order/review/ticket rows
 * when no product image applies (`imageUrl` absent; see lib/queue.ts, which
 * documents WHY that's a permanent fallback, not a temporary shim).
 *
 * There is no monogram component anywhere else in this app (checked before
 * writing this). Kept local to QueueRow rather than promoted to
 * components/ui/Thumb.tsx or components/ui/ — QueueRow is currently the
 * only caller, and CustomerRow.tsx already has its own inline 40pt avatar
 * that isn't this component's concern to unify. Promote this to
 * components/ui if a second caller needs the same 60pt tile.
 *
 * Sized AND radiused identically to Thumb's "list" box (`theme.thumb.list`
 * 60pt, `theme.radii.md`) so the two fallback paths (product photo vs.
 * customer monogram) occupy the exact same slot and read as one column. It
 * was a full circle (`thumb.list / 2`), which put circles and rounded squares
 * side by side in a single list — and made an order that happened to carry an
 * `image_url` render a square between circles. Fixed HERE, on the monogram:
 * Thumb is the shape the rest of the app's lists already use. Never moss —
 * this app's one accent is already spent on the
 * Dashboard's revenue chart and the Approve swipe action; the monogram uses
 * the same neutral surface tint CustomerRow's avatar uses.
 */
function Monogram({ label, testID }: { label: string; testID?: string }) {
  return (
    <View style={styles.monogram} accessible={false} testID={testID}>
      <Text preset="bodyEmphasis" color="text">
        {monogramInitial(label)}
      </Text>
    </View>
  );
}

/**
 * A "needs you" queue row — see `buildQueue` (lib/queue.ts) for how items
 * are sourced, ordered and capped. Renders two shapes:
 *
 *  - A normal queue item: 60pt Thumb (or the Monogram fallback above), 17pt
 *    primary (bodyEmphasis), 13pt secondary (caption), and a right column
 *    with a serif amount + typed StatusBadge.
 *  - A "See all N" overflow row (`item.kind === "seeAll"` — see lib/queue.ts's
 *    QueueItem doc for why this is an explicit discriminant rather than
 *    `badgeTone === undefined`): single-line, no thumb/monogram/badge/amount
 *    — it's a navigational affordance, not a queue entry.
 *
 * Deliberately NOT wrapped in SwipeRow — the brief is explicit that the
 * CALLER wraps it (Task 8's Dashboard screen), since swipe actions differ
 * per item type (orders: Approve/Cancel; reviews: Approve/Reject; tickets:
 * Close only; stock: no swipe) and this same row will be reused for other
 * lists in increment 3.
 */
export function QueueRow({ item, onPress }: QueueRowProps) {
  const isSeeAll = item.kind === "seeAll";

  if (isSeeAll) {
    return (
      <PressableRow
        lines={1}
        onPress={onPress}
        testID={`queue-row-${item.id}`}
        accessibilityLabel={item.primary}
        accessibilityRole="link"
      >
        {/* Ink, NOT accent. On the Dashboard the one moss accent is spent on
            exactly two things — the revenue chart and the Approve swipe — and
            a moss "See all" link was a third (caught on device, Task 8). The
            chevron already carries the affordance; the colour doesn't need
            to. */}
        <Text preset="label" color="text" style={styles.seeAllLabel} numberOfLines={1}>
          {item.primary}
        </Text>
        <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
      </PressableRow>
    );
  }

  // Product photo applies to orders (first line item's image) and low
  // stock (the variant's product image) — both are genuinely product
  // thumbnails. Reviews and tickets never carry one (see lib/queue.ts).
  // Low stock has no customer to monogram, so its fallback is Thumb's own
  // built-in placeholder, not the customer disc.
  const showThumb = Boolean(item.imageUrl) || item.type === "stock";

  const a11yParts = [item.primary, item.secondary];
  if (item.amount) a11yParts.push(item.amount);
  if (item.badgeLabel) a11yParts.push(item.badgeLabel);

  return (
    <PressableRow
      lines={2}
      onPress={onPress}
      testID={`queue-row-${item.id}`}
      accessibilityLabel={a11yParts.join(", ")}
    >
      {showThumb ? (
        <Thumb
          uri={item.imageUrl}
          recyclingKey={item.id}
          testID={`queue-row-${item.id}-thumb`}
        />
      ) : (
        <Monogram label={item.primary} testID={`queue-row-${item.id}-monogram`} />
      )}

      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {item.primary}
        </Text>
        <Text preset="caption" color="textTertiary" numberOfLines={1}>
          {item.secondary}
        </Text>
      </View>

      <View style={styles.trailing}>
        {item.amount ? (
          <Text preset="h3" color="text" style={styles.amount} numberOfLines={1}>
            {item.amount}
          </Text>
        ) : null}
        {item.badgeLabel ? <StatusBadge label={item.badgeLabel} tone={item.badgeTone} /> : null}
      </View>
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  // Neutral surface — never moss (see the Monogram doc comment above).
  // Deliberately `sink`, not CustomerRow's `surfaceAlt`: this row (unlike
  // CustomerRow, which always sits inside a white elevated Card) can sit
  // directly on the Paper background, and surfaceAlt (#FAF8F2) is nearly
  // invisible against Paper (#F7F6F2) — confirmed on-device. `sink` is the
  // same token Thumb's own built-in placeholder uses, so both "no photo"
  // fallbacks (product placeholder, customer monogram) read with equal
  // visibility in the same row context.
  //
  // `borderColor: textTertiary` is deliberate, not decorative: PressableRow's
  // iOS pressed state (components/ui/PressableRow.tsx) repaints the row
  // background to this SAME `sink` token, so a fill-only disc has zero
  // contrast against its own row while held — the disc's edge vanishes and
  // the initial appears to float. textTertiary (#5C5953) holds ~5.8:1
  // against `sink` (and ~6.5:1 against Paper at rest), so the ring stays
  // visible in both states. Fixed here, not in PressableRow or Thumb — a
  // shared primitive's default press token is not the place for a one-row
  // fix (see inc2-task-7-report.md, "Fix round 1").
  monogram: {
    width: theme.thumb.list,
    height: theme.thumb.list,
    // Thumb's radius, not a circle — one leading-art shape per list.
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.sink,
    borderWidth: theme.hairline,
    borderColor: theme.colors.textTertiary,
    alignItems: "center",
    justifyContent: "center",
    flexShrink: 0,
  },
  info: { flex: 1, gap: 3, minWidth: 0 },
  trailing: { alignItems: "flex-end", gap: 4 },
  amount: { fontVariant: ["tabular-nums"] },
  seeAllLabel: { flex: 1 },
});

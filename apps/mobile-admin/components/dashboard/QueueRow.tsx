import { View, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { Monogram, PressableRow, StatusBadge, Text, Thumb } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { QueueItem } from "@/lib/queue";

interface QueueRowProps {
  item: QueueItem;
  onPress: () => void;
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
  // The monogram tile itself now lives in components/ui/Monogram.tsx — the
  // size/radius/fill/ring reasoning moved with it. It is drawn at the default
  // size, which is `theme.thumb.list`, so the photo and no-photo paths still
  // occupy the identical slot.
  info: { flex: 1, gap: 3, minWidth: 0 },
  trailing: { alignItems: "flex-end", gap: 4 },
  amount: { fontVariant: ["tabular-nums"] },
  seeAllLabel: { flex: 1 },
});

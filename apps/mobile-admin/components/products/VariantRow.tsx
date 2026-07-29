import { useState } from "react";
import { Pressable, View, StyleSheet } from "react-native";
import Animated, { useReducedMotion } from "react-native-reanimated";
import { ChevronDown } from "lucide-react-native";
import { Text, StatusBadge, type StatusTone } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/product-display";
import { disclosureEntering, disclosureExiting, useChevronRotationStyle } from "./disclosure-motion";
import { VariantEditor, variantLabel } from "./VariantEditor";
import type { ProductVariant } from "@repo/mobile-shared/api/schemas/products";
import type { UpdateVariantBody } from "@repo/mobile-shared/api/products";

/**
 * At or below this stock count a variant reads as "Low" rather than healthy —
 * a named constant so the threshold lives in one place instead of scattered
 * magic numbers across the summary row and any future stock UI.
 */
export const LOW_STOCK = 5;

/**
 * Stock count -> badge tone + label. Pure, so the boundary is cheap to pin exactly.
 *
 * The healthy (>LOW_STOCK) case does NOT use tone "success", and the reason
 * is NOT that a moss-tint success badge is forbidden. The spec's Guardrails
 * permit them generally — "Success stays a moss tint (#E8EEE2/#2D4A2B), never
 * a solid moss fill" — which is why `ReviewStatusBadge` maps approved →
 * success and `OrderStatusBadges` maps fulfilled → success, both correctly.
 * The one-accent-per-view constraint the spec states alongside it is scoped
 * to the DASHBOARD, where the moss is spent on the chart and the Approve
 * swipe (see `lib/queue.ts`, which therefore emits no `success` tone at all).
 *
 * The reason here is informational, not chromatic: "healthy" is the default
 * state of every variant on a healthy product, and a badge on every row that
 * says nothing actionable is noise the eye has to filter. Only the actionable
 * middle state earns a tone — the functional amber "warning". Healthy and
 * out-of-stock both read "muted" (quiet, no accent), and the LABEL text, not
 * the badge colour, distinguishes them.
 */
export function stockTone(quantity: number): { tone: StatusTone; label: string } {
  if (quantity === 0) return { tone: "muted", label: "Out of stock" };
  if (quantity <= LOW_STOCK) return { tone: "warning", label: `Low: ${quantity}` };
  return { tone: "muted", label: `${quantity} in stock` };
}

interface VariantRowProps {
  variant: ProductVariant;
  onUpdate: (variantId: string, body: UpdateVariantBody) => void;
  /** Auto-expand on mount — the sole-variant product doesn't need a tap to see its own price/stock/SKU. */
  defaultOpen?: boolean;
}

/**
 * Collapsible per-variant summary (label, price · stock · SKU caption, stock
 * badge, rotating chevron) that expands into today's full VariantEditor body
 * — collapsing a dense 7-field wall behind a single 44-56pt tap target.
 */
export function VariantRow({ variant, onUpdate, defaultOpen = false }: VariantRowProps) {
  const [open, setOpen] = useState(defaultOpen);
  const reduceMotion = useReducedMotion();
  const chevronStyle = useChevronRotationStyle(open, reduceMotion);

  // The wire has no variant name — a product's sole variant carries no option
  // tuple at all, so "Default variant" reads better than repeating its SKU
  // (which is already in the caption below).
  const isDefaultVariant = variant.option_values.length === 0;
  const label = isDefaultVariant ? "Default variant" : variantLabel(variant);
  const caption = `${formatMoney(variant.price, variant.currency_code)} · ${variant.inventory_quantity} in stock · ${variant.sku}`;
  const stock = stockTone(variant.inventory_quantity);

  return (
    <View>
      <Pressable
        onPress={() => setOpen((current) => !current)}
        accessibilityRole="button"
        accessibilityState={{ expanded: open }}
        accessibilityLabel={`${label}, ${caption}, ${open ? "expanded" : "collapsed"}`}
        style={styles.summary}
      >
        <View style={styles.summaryText}>
          <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
            {label}
          </Text>
          <Text preset="caption" color="textTertiary" numberOfLines={1}>
            {caption}
          </Text>
        </View>
        <View style={styles.summaryRight}>
          <StatusBadge label={stock.label} tone={stock.tone} />
          <Animated.View style={chevronStyle}>
            <ChevronDown size={18} color={theme.colors.textSecondary} strokeWidth={2} />
          </Animated.View>
        </View>
      </Pressable>
      {open ? (
        <Animated.View
          testID="variant-row-body"
          entering={reduceMotion ? undefined : disclosureEntering}
          exiting={reduceMotion ? undefined : disclosureExiting}
        >
          <VariantEditor variant={variant} onUpdate={onUpdate} />
        </Animated.View>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  summary: {
    minHeight: theme.row.minHeightSingle,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: theme.spacing.sm,
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.row.paddingH,
  },
  summaryText: {
    flex: 1,
    gap: 2,
  },
  summaryRight: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.sm,
  },
});

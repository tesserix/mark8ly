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
 * Controller decision: the healthy (>LOW_STOCK) case does NOT use tone
 * "success" — that tone's text IS moss (on a moss tint field). The design
 * system's one-moss-accent-per-view rule is non-negotiable and outranks the
 * spec's tone enumeration — a healthy-stock product would otherwise show a
 * moss badge on every variant row alongside the header's moss "Save",
 * spending the accent many times per screen. Healthy and out-of-stock both
 * read as "muted" (quiet, no accent); only the actionable low-stock middle
 * state draws the functional amber "warning" tone. Label text — not badge
 * color — distinguishes the two muted states.
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
    minHeight: 56,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: theme.spacing.sm,
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.lg,
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

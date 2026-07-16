import { useState } from "react";
import { View, TextInput, StyleSheet } from "react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { ProductVariant } from "@repo/mobile-shared/api/schemas/products";
import type { UpdateVariantBody } from "@repo/mobile-shared/api/products";

/**
 * The wire has no variant name. A variant is described by its option values
 * ("M / Blue"); the SKU — which every real variant has — is the honest fallback.
 */
export function variantLabel(variant: ProductVariant): string {
  if (variant.option_values.length > 0) {
    return variant.option_values.map((o) => o.value).join(" / ");
  }
  return variant.sku;
}

function FieldLabel({ label }: { label: string }) {
  return (
    <Text preset="caption" color="textTertiary">
      {label}
    </Text>
  );
}

interface NumericFieldProps {
  label: string;
  accessibilityLabel: string;
  initial: number | undefined;
  integer?: boolean;
  onCommit: (value: number) => void;
}

/**
 * Commits on blur, never on change — a PATCH per keystroke would hammer the
 * 60 req/min per-user rate limiter on the mobile routes. Silently does nothing
 * for unparseable or unchanged input: sending NaN would be a silent data loss,
 * which is the bug class this project exists to kill.
 */
function NumericField({ label, accessibilityLabel, initial, integer, onCommit }: NumericFieldProps) {
  const [text, setText] = useState(initial === undefined ? "" : String(initial));

  const handleBlur = () => {
    const trimmed = text.trim();
    if (trimmed === "") return;
    const parsed = integer ? parseInt(trimmed, 10) : parseFloat(trimmed);
    if (Number.isNaN(parsed)) return;
    if (parsed === initial) return;
    onCommit(parsed);
  };

  return (
    <View style={styles.field}>
      <FieldLabel label={label} />
      <TextInput
        style={styles.input}
        value={text}
        onChangeText={setText}
        onBlur={handleBlur}
        keyboardType="decimal-pad"
        accessibilityLabel={accessibilityLabel}
      />
    </View>
  );
}

interface VariantEditorProps {
  variant: ProductVariant;
  onUpdate: (variantId: string, body: UpdateVariantBody) => void;
}

export function VariantEditor({ variant, onUpdate }: VariantEditorProps) {
  const [sku, setSku] = useState(variant.sku);

  const handleSkuBlur = () => {
    const trimmed = sku.trim();
    // SKU is `binding:"required,max=100"` on the wire — an empty one is a 400.
    if (trimmed === "" || trimmed === variant.sku) return;
    onUpdate(variant.id, { sku: trimmed });
  };

  return (
    <View style={styles.root}>
      <Text preset="bodyEmphasis" color="text">
        {variantLabel(variant)}
      </Text>

      <View style={styles.field}>
        <FieldLabel label="SKU" />
        <TextInput
          style={styles.input}
          value={sku}
          onChangeText={setSku}
          onBlur={handleSkuBlur}
          autoCapitalize="characters"
          accessibilityLabel="SKU"
        />
      </View>

      <View style={styles.row}>
        <NumericField
          label={`Price (${variant.currency_code})`}
          accessibilityLabel="Price"
          initial={variant.price}
          onCommit={(price) => onUpdate(variant.id, { price })}
        />
        <NumericField
          label="Stock"
          accessibilityLabel="Stock"
          initial={variant.inventory_quantity}
          integer
          onCommit={(inventory_quantity) => onUpdate(variant.id, { inventory_quantity })}
        />
      </View>

      <Text preset="caption" color="textTertiary">
        Shipping
      </Text>
      <View style={styles.row}>
        <NumericField
          label="Weight (g)"
          accessibilityLabel="Weight in grams"
          initial={variant.weight_grams}
          integer
          onCommit={(weight_grams) => onUpdate(variant.id, { weight_grams })}
        />
        <NumericField
          label="Length (cm)"
          accessibilityLabel="Length in centimetres"
          initial={variant.length_cm}
          onCommit={(length_cm) => onUpdate(variant.id, { length_cm })}
        />
      </View>
      <View style={styles.row}>
        <NumericField
          label="Width (cm)"
          accessibilityLabel="Width in centimetres"
          initial={variant.width_cm}
          onCommit={(width_cm) => onUpdate(variant.id, { width_cm })}
        />
        <NumericField
          label="Height (cm)"
          accessibilityLabel="Height in centimetres"
          initial={variant.height_cm}
          onCommit={(height_cm) => onUpdate(variant.id, { height_cm })}
        />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { gap: theme.spacing.sm, paddingVertical: theme.spacing.md },
  row: { flexDirection: "row", gap: theme.spacing.md },
  field: { flex: 1, gap: theme.spacing.xs },
  input: {
    height: 44,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.sm,
    color: theme.colors.text,
    backgroundColor: theme.colors.elevated,
  },
});

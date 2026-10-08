import {
  FlatList,
  StyleSheet,
  TouchableOpacity,
  View,
} from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { Minus, Plus, ShoppingBag, Trash2 } from "lucide-react-native";
import { useCartStore, type CartLine, lineKey } from "@/lib/cart-store";
import { describeCartPersonalisation } from "@/lib/personalisation";
import { Button, EmptyState, Hairline, PageHeader, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/format";

export default function CartScreen() {
  const router = useRouter();
  const lines = useCartStore((s) => s.lines);
  const subtotal = useCartStore((s) => s.subtotalAmount());
  const setQuantity = useCartStore((s) => s.setQuantity);
  const remove = useCartStore((s) => s.remove);

  const currency = lines[0]?.currencyCode ?? "USD";

  return (
    <Screen>
      <PageHeader
        eyebrow="CART"
        title="Your bag"
        subtitle={
          lines.length
            ? `${lines.reduce((s, l) => s + l.quantity, 0)} item${lines.length === 1 ? "" : "s"}`
            : undefined
        }
      />

      {lines.length === 0 ? (
        <View style={styles.emptyWrap}>
          <EmptyState
            icon={<ShoppingBag size={28} color={theme.colors.textTertiary} strokeWidth={1.5} />}
            title="Your bag is empty"
            message="Browse the shop to add something you love."
            action={<Button label="Start shopping" onPress={() => router.push("/shop")} />}
          />
        </View>
      ) : (
        <>
          <FlatList
            data={lines}
            keyExtractor={(l) => lineKey(l)}
            ItemSeparatorComponent={() => <Hairline inset={theme.spacing.lg} />}
            renderItem={({ item }) => {
              // One key for the list and for the store (#964). A variant
              // can appear on several lines once personalisation ships, so
              // keying either on variantId alone would collide.
              const key = lineKey(item);
              return (
                <CartRow
                  line={item}
                  currency={currency}
                  onIncrement={() => setQuantity(key, item.quantity + 1)}
                  onDecrement={() => setQuantity(key, item.quantity - 1)}
                  onRemove={() => remove(key)}
                />
              );
            }}
            contentContainerStyle={styles.list}
          />

          <View style={styles.summary}>
            <Hairline />
            <View style={styles.summaryRow}>
              <Text preset="body" color="textSecondary">
                Subtotal
              </Text>
              <Text preset="price" color="text">
                {formatMoney(subtotal, currency)}
              </Text>
            </View>
            <Text preset="caption" color="textTertiary">
              Shipping and taxes calculated at checkout.
            </Text>
            <Button
              label="Checkout"
              onPress={() => router.push("/checkout")}
              fullWidth
              style={{ marginTop: theme.spacing.md }}
            />
          </View>
        </>
      )}
    </Screen>
  );
}

function CartRow({
  line,
  currency,
  onIncrement,
  onDecrement,
  onRemove,
}: {
  line: CartLine;
  currency: string;
  onIncrement: () => void;
  onDecrement: () => void;
  onRemove: () => void;
}) {
  return (
    <View style={styles.row}>
      <View style={styles.thumb}>
        {line.imageUrl ? (
          <Image source={{ uri: line.imageUrl }} style={{ width: "100%", height: "100%" }} contentFit="cover" />
        ) : null}
      </View>
      <View style={styles.body}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={2}>
          {line.title}
        </Text>
        {line.variantTitle ? (
          <Text preset="caption" color="textTertiary">
            {line.variantTitle}
          </Text>
        ) : null}
        {/* What the buyer asked for (#969), from the snapshots on the line.
            A photo is named, not shown: its preview is a ten-minute signed
            URL that would be dead by the time they scrolled here. */}
        {describeCartPersonalisation(line.personalisation).map((p, i) => (
          <Text key={`${p.label}-${i}`} preset="caption" color="textSecondary" numberOfLines={2}>
            {p.label}: {p.value}
          </Text>
        ))}
        <Text preset="price" color="text">
          {formatMoney(Number(line.unitPriceAmount) * line.quantity, currency)}
        </Text>
        <View style={styles.qtyRow}>
          <View style={styles.qtyControl}>
            <TouchableOpacity onPress={onDecrement} style={styles.qtyBtn} hitSlop={6} accessibilityLabel="Decrease quantity">
              <Minus size={14} color={theme.colors.text} strokeWidth={1.75} />
            </TouchableOpacity>
            <Text preset="bodyEmphasis" color="text" style={{ minWidth: 24, textAlign: "center" }}>
              {line.quantity}
            </Text>
            <TouchableOpacity onPress={onIncrement} style={styles.qtyBtn} hitSlop={6} accessibilityLabel="Increase quantity">
              <Plus size={14} color={theme.colors.text} strokeWidth={1.75} />
            </TouchableOpacity>
          </View>
          <TouchableOpacity onPress={onRemove} hitSlop={8} accessibilityLabel="Remove item">
            <Trash2 size={16} color={theme.colors.danger} strokeWidth={1.75} />
          </TouchableOpacity>
        </View>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  emptyWrap: { flex: 1, justifyContent: "center" },
  list: { paddingHorizontal: theme.spacing.lg, paddingBottom: theme.spacing.lg },
  row: {
    flexDirection: "row",
    gap: theme.spacing.md,
    paddingVertical: theme.spacing.md,
  },
  thumb: {
    width: 88,
    height: 88,
    borderRadius: theme.radii.md,
    overflow: "hidden",
    backgroundColor: theme.colors.surfaceAlt,
  },
  body: { flex: 1, gap: 4 },
  qtyRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    marginTop: theme.spacing.xs,
  },
  qtyControl: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.sm,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    borderRadius: theme.radii.pill,
    paddingVertical: 4,
    paddingHorizontal: theme.spacing.sm,
  },
  qtyBtn: {
    width: 28,
    height: 28,
    alignItems: "center",
    justifyContent: "center",
  },
  summary: {
    padding: theme.spacing.lg,
    gap: theme.spacing.sm,
    backgroundColor: theme.colors.background,
  },
  summaryRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingTop: theme.spacing.md,
  },
});

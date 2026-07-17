import { useCallback, useMemo, useState } from "react";
import {
  View,
  ScrollView,
  Pressable,
  Switch,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { ApiError } from "@repo/mobile-shared/api/client";
import { COUPON_TYPES } from "@repo/mobile-shared/api/schemas/coupons";
import { useCreateCoupon } from "@/lib/admin-api/coupon-actions";
import { BackHeader, FieldInput, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

const TYPE_LABEL: Record<string, string> = {
  percentage: "% off",
  fixed_amount: "Amount off",
  free_shipping: "Free shipping",
};

function parseOptionalNumber(raw: string): number | undefined {
  const t = raw.trim();
  if (t === "") return undefined;
  const n = Number(t);
  return Number.isFinite(n) ? n : undefined;
}

export default function NewCouponScreen() {
  const router = useRouter();
  const create = useCreateCoupon();

  const [code, setCode] = useState("");
  const [title, setTitle] = useState("");
  const [type, setType] = useState<string>("percentage");
  const [value, setValue] = useState("");
  const [minPurchase, setMinPurchase] = useState("");
  const [usageLimit, setUsageLimit] = useState("");
  const [perCustomer, setPerCustomer] = useState("");
  const [stackable, setStackable] = useState(false);
  const [description, setDescription] = useState("");

  const needsValue = type !== "free_shipping";
  const valueNum = parseOptionalNumber(value);
  const canSubmit = useMemo(() => {
    if (!code.trim() || !title.trim()) return false;
    if (needsValue && (valueNum === undefined || valueNum <= 0)) return false;
    return !create.isPending;
  }, [code, title, needsValue, valueNum, create.isPending]);

  const onSubmit = useCallback(() => {
    if (!canSubmit) return;
    create.mutate(
      {
        code: code.trim(),
        title: title.trim(),
        type,
        // free_shipping ignores value; send 0 so the required decimal is present.
        value: needsValue ? (valueNum ?? 0) : 0,
        ...(description.trim() ? { description: description.trim() } : {}),
        ...(parseOptionalNumber(minPurchase) !== undefined
          ? { min_purchase: parseOptionalNumber(minPurchase) }
          : {}),
        ...(parseOptionalNumber(usageLimit) !== undefined
          ? { usage_limit: parseOptionalNumber(usageLimit) }
          : {}),
        ...(parseOptionalNumber(perCustomer) !== undefined
          ? { per_customer: parseOptionalNumber(perCustomer) }
          : {}),
        stackable,
      },
      {
        onSuccess: () => router.back(),
        onError: (err) =>
          Alert.alert(
            "Couldn't create coupon",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      },
    );
  }, [
    canSubmit,
    create,
    code,
    title,
    type,
    needsValue,
    valueNum,
    description,
    minPurchase,
    usageLimit,
    perCustomer,
    stackable,
    router,
  ]);

  return (
    <Screen>
      <BackHeader eyebrow="MARKETING" title="New coupon" />
      <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
        <FieldInput
          label="Code"
          value={code}
          onChangeText={(t) => setCode(t.toUpperCase())}
          placeholder="SUMMER10"
          autoCapitalize="characters"
          autoCorrect={false}
        />
        <FieldInput label="Title" value={title} onChangeText={setTitle} placeholder="Summer sale" />

        <Text preset="caption" color="textTertiary">
          Type
        </Text>
        <View style={styles.chipRow}>
          {COUPON_TYPES.map((t) => (
            <Pressable
              key={t}
              style={[styles.chip, t === type && styles.chipSelected]}
              onPress={() => setType(t)}
              accessibilityRole="button"
              accessibilityState={{ selected: t === type }}
              accessibilityLabel={TYPE_LABEL[t]}
            >
              <Text preset="caption" color={t === type ? "inverse" : "text"}>
                {TYPE_LABEL[t]}
              </Text>
            </Pressable>
          ))}
        </View>

        {needsValue ? (
          <FieldInput
            label={type === "percentage" ? "Percentage" : "Amount"}
            value={value}
            onChangeText={setValue}
            placeholder={type === "percentage" ? "10" : "15"}
            keyboardType="decimal-pad"
          />
        ) : null}

        <FieldInput
          label="Minimum spend (optional)"
          value={minPurchase}
          onChangeText={setMinPurchase}
          placeholder="No minimum"
          keyboardType="decimal-pad"
        />
        <FieldInput
          label="Total usage limit (optional)"
          value={usageLimit}
          onChangeText={setUsageLimit}
          placeholder="Unlimited"
          keyboardType="number-pad"
        />
        <FieldInput
          label="Per-customer limit (optional)"
          value={perCustomer}
          onChangeText={setPerCustomer}
          placeholder="Unlimited"
          keyboardType="number-pad"
        />

        <View style={styles.switchRow}>
          <Text preset="body" color="text">
            Stackable with other discounts
          </Text>
          <Switch
            value={stackable}
            onValueChange={setStackable}
            trackColor={{ true: theme.colors.accent }}
          />
        </View>

        <FieldInput
          label="Description (optional)"
          value={description}
          onChangeText={setDescription}
          placeholder="Shown on the storefront"
          multiline
        />

        <Text preset="caption" color="textTertiary">
          New coupons are active immediately with no end date. Set scheduling and targeting from the
          web dashboard.
        </Text>

        <Pressable
          style={[styles.primaryBtn, !canSubmit && styles.disabled]}
          onPress={onSubmit}
          disabled={!canSubmit}
          accessibilityRole="button"
          accessibilityLabel="Create coupon"
        >
          {create.isPending ? (
            <ActivityIndicator size="small" color={theme.colors.inverse} />
          ) : (
            <Text preset="bodyEmphasis" color="inverse">
              Create coupon
            </Text>
          )}
        </Pressable>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: theme.spacing.lg, gap: theme.spacing.md, paddingBottom: theme.spacing.huge },
  chipRow: { flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.xs },
  chip: {
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.md,
    minHeight: theme.touchTarget,
    justifyContent: "center",
  },
  chipSelected: { backgroundColor: theme.colors.text, borderColor: theme.colors.text },
  switchRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: theme.spacing.md,
  },
  primaryBtn: {
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing.sm,
  },
  disabled: { opacity: 0.4 },
});

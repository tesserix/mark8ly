import { View, TouchableOpacity, StyleSheet } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { Text, StatusBadge } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatCouponValue, couponStatusTone } from "@/lib/coupon-display";
import type { Coupon } from "@repo/mobile-shared/api/types";

interface CouponRowProps {
  coupon: Coupon;
  currency: string;
  onPress: (coupon: Coupon) => void;
}

function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export function CouponRow({ coupon, currency, onPress }: CouponRowProps) {
  return (
    <TouchableOpacity
      style={styles.container}
      onPress={() => onPress(coupon)}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={`Coupon ${coupon.code}, ${coupon.status}`}
    >
      <View style={styles.info}>
        <View style={styles.topRow}>
          <Text preset="bodyEmphasis" color="text" style={styles.code} numberOfLines={1}>
            {coupon.code}
          </Text>
          <StatusBadge label={titleize(coupon.status)} tone={couponStatusTone(coupon.status)} />
        </View>
        <Text preset="caption" color="textSecondary" numberOfLines={1}>
          {formatCouponValue(coupon, currency)} · {coupon.title}
        </Text>
      </View>
      <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  container: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
    gap: theme.spacing.md,
  },
  info: { flex: 1, gap: 4 },
  topRow: { flexDirection: "row", alignItems: "center", gap: theme.spacing.sm },
  code: { flexShrink: 1, fontVariant: ["tabular-nums"] },
});

import {
  RefreshControl,
  ScrollView,
  View,
  StyleSheet,
  TouchableOpacity,
} from "react-native";
import { useRouter } from "expo-router";
import { ChevronRight } from "lucide-react-native";
import { useDashboard } from "@/lib/hooks/use-dashboard";
import { DashboardStats } from "@/components/DashboardStats";
import {
  Card,
  EmptyState,
  Hairline,
  PageHeader,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type { RecentOrder, LowStockItem } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

function todaysDate() {
  return new Date().toLocaleDateString("en-US", {
    weekday: "long",
    month: "long",
    day: "numeric",
  });
}

function formatCurrency(amount: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
  }).format(amount);
}

function ListRow({
  primary,
  secondary,
  trailing,
  trailingTone = "text",
  onPress,
  accessibilityLabel,
}: {
  primary: string;
  secondary?: string;
  trailing: string;
  trailingTone?: "text" | "danger" | "accent";
  onPress: () => void;
  accessibilityLabel: string;
}) {
  return (
    <TouchableOpacity
      style={styles.row}
      onPress={onPress}
      activeOpacity={0.6}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
    >
      <View style={styles.rowMain}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {primary}
        </Text>
        {secondary ? (
          <Text preset="caption" color="textTertiary" numberOfLines={1}>
            {secondary}
          </Text>
        ) : null}
      </View>
      <Text preset="bodyEmphasis" color={trailingTone}>
        {trailing}
      </Text>
      <ChevronRight
        size={16}
        color={theme.colors.textTertiary}
        strokeWidth={1.75}
        style={styles.chevron}
      />
    </TouchableOpacity>
  );
}

function Section({
  title,
  onSeeAll,
  children,
}: {
  title: string;
  onSeeAll?: () => void;
  children: React.ReactNode;
}) {
  return (
    <View style={styles.section}>
      <View style={styles.sectionHeader}>
        <Text preset="eyebrow" color="textTertiary">
          {title}
        </Text>
        {onSeeAll ? (
          <TouchableOpacity
            onPress={onSeeAll}
            accessibilityRole="link"
            accessibilityLabel={`View all ${title.toLowerCase()}`}
            hitSlop={8}
          >
            <Text preset="caption" color="accent">
              See all
            </Text>
          </TouchableOpacity>
        ) : null}
      </View>
      <Card padding={0}>{children}</Card>
    </View>
  );
}

export default function DashboardScreen() {
  const dockPad = useDockClearance();
  const { data, isLoading, refetch, isRefetching } = useDashboard();
  const router = useRouter();

  return (
    <Screen>
      <ScrollView
        contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}
        refreshControl={
          <RefreshControl
            refreshing={isRefetching}
            onRefresh={refetch}
            tintColor={theme.colors.text}
          />
        }
      >
        <PageHeader
          eyebrow="DASHBOARD"
          title={todaysDate()}
          subtitle={data ? "Snapshot of your store today." : undefined}
        />

        {!data ? (
          <View style={styles.contentPad}>
            {isLoading ? (
              <EmptyState title="Loading…" message="Fetching your latest numbers." />
            ) : (
              <EmptyState
                title="Couldn't load dashboard"
                message="Pull to refresh or check your connection."
              />
            )}
          </View>
        ) : (
          <View style={styles.contentPad}>
            <DashboardStats stats={data.stats} />

            {data.recent_orders.length > 0 ? (
              <Section
                title="Recent Orders"
                onSeeAll={() => router.push("/(tabs)/orders")}
              >
                {data.recent_orders.map((order, i) => (
                  <RowWrapper key={order.id} divider={i > 0}>
                    <RecentOrderRow
                      order={order}
                      onPress={() => router.push(`/(tabs)/orders/${order.id}`)}
                    />
                  </RowWrapper>
                ))}
              </Section>
            ) : null}

            {data.low_stock.length > 0 ? (
              <Section title="Low Stock">
                {data.low_stock.map((item, i) => (
                  <RowWrapper key={item.id} divider={i > 0}>
                    <LowStockRow
                      item={item}
                      onPress={() => router.push(`/(tabs)/products/${item.id}`)}
                    />
                  </RowWrapper>
                ))}
              </Section>
            ) : null}

            {data.top_products.length > 0 ? (
              <Section title="Top Products">
                {data.top_products.map((p, i) => (
                  <RowWrapper key={p.id} divider={i > 0}>
                    <ListRow
                      primary={p.title}
                      secondary={`${p.units_sold} sold`}
                      trailing={formatCurrency(p.revenue)}
                      onPress={() => router.push(`/(tabs)/products/${p.id}`)}
                      accessibilityLabel={`${p.title}, ${p.units_sold} sold, ${formatCurrency(p.revenue)} revenue`}
                    />
                  </RowWrapper>
                ))}
              </Section>
            ) : null}
          </View>
        )}
      </ScrollView>
    </Screen>
  );
}

function RowWrapper({ children, divider }: { children: React.ReactNode; divider: boolean }) {
  return (
    <View>
      {divider ? <Hairline inset={theme.spacing.lg} /> : null}
      {children}
    </View>
  );
}

function RecentOrderRow({
  order,
  onPress,
}: {
  order: RecentOrder;
  onPress: () => void;
}) {
  return (
    <ListRow
      primary={`#${order.order_number}`}
      secondary={order.customer_email}
      trailing={formatCurrency(order.grand_total)}
      onPress={onPress}
      accessibilityLabel={`Order ${order.order_number}, ${order.customer_email}, ${formatCurrency(order.grand_total)}`}
    />
  );
}

function LowStockRow({ item, onPress }: { item: LowStockItem; onPress: () => void }) {
  return (
    <ListRow
      primary={item.title}
      trailing={`${item.quantity} left`}
      trailingTone="danger"
      onPress={onPress}
      accessibilityLabel={`${item.title}, ${item.quantity} left in stock`}
    />
  );
}

const styles = StyleSheet.create({
  scroll: {
    paddingBottom: theme.spacing.huge,
  },
  contentPad: {
    paddingHorizontal: theme.spacing.lg,
    gap: theme.spacing.xl,
  },
  section: { gap: theme.spacing.xs },
  sectionHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingHorizontal: theme.spacing.xs,
    paddingBottom: theme.spacing.xs,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    gap: theme.spacing.md,
    minHeight: 56,
  },
  rowMain: { flex: 1, gap: 2 },
  chevron: { marginLeft: -theme.spacing.xs },
});

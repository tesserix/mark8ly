import {
  RefreshControl,
  ScrollView,
  View,
  StyleSheet,
  TouchableOpacity,
} from "react-native";
import { useRouter } from "expo-router";
import { ChevronRight } from "lucide-react-native";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useDashboard } from "@/lib/hooks/use-dashboard";
import { DashboardStats } from "@/components/DashboardStats";
import { NotificationBell } from "@/components/dashboard/NotificationBell";
import { DashboardOrderRow } from "@/components/dashboard/DashboardOrderRow";
import {
  Card,
  EmptyState,
  Hairline,
  PageHeader,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import type {
  RecentOrder,
  LowStockItem,
  TopProduct,
  SetupChecklist,
} from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

function todaysDate() {
  return new Date().toLocaleDateString("en-US", {
    weekday: "long",
    month: "long",
    day: "numeric",
  });
}

const CHECKLIST_ITEMS = 8;

function checklistComplete(c: SetupChecklist): number {
  return [
    c.has_store,
    c.has_brand_assets,
    c.has_product,
    c.has_storefront_theme,
    c.has_payment_provider,
    c.has_shipping_carrier,
    c.has_return_policy,
    c.has_custom_domain,
  ].filter(Boolean).length;
}

function ListRow({
  primary,
  secondary,
  trailing,
  trailingTone = "text",
  leading,
  onPress,
  accessibilityLabel,
}: {
  primary: string;
  secondary?: string;
  trailing: string;
  trailingTone?: "text" | "danger" | "accent";
  leading?: React.ReactNode;
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
      {leading ? <View style={styles.leading}>{leading}</View> : null}
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
  seeAllLabel = "See all",
  children,
}: {
  title: string;
  onSeeAll?: () => void;
  seeAllLabel?: string;
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
            accessibilityLabel={seeAllLabel}
            hitSlop={8}
          >
            <Text preset="caption" color="accent">
              {seeAllLabel}
            </Text>
          </TouchableOpacity>
        ) : null}
      </View>
      <Card padding={0}>{children}</Card>
    </View>
  );
}

function RowWrapper({
  children,
  divider,
}: {
  children: React.ReactNode;
  divider: boolean;
}) {
  return (
    <View>
      {divider ? <Hairline inset={theme.spacing.lg} /> : null}
      {children}
    </View>
  );
}

export default function DashboardScreen() {
  const dockPad = useDockClearance();
  const { data, isLoading, refetch, isRefetching } = useDashboard();
  const router = useRouter();
  const currencyCode = useTenantStore((s) => s.activeStore?.currency_code);

  const pendingOrders = data?.recent_orders.filter((o) => o.status === "pending") ?? [];
  const otherRecent = data?.recent_orders.filter((o) => o.status !== "pending") ?? [];
  const morePending = data ? data.stats.orders_pending - pendingOrders.length : 0;
  const setupDone = data ? checklistComplete(data.setup_checklist) : CHECKLIST_ITEMS;

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
          rightSlot={<NotificationBell />}
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
            <DashboardStats stats={data.stats} currencyCode={currencyCode} />

            {pendingOrders.length > 0 ? (
              <Section
                title="Awaiting approval"
                onSeeAll={morePending > 0 ? () => router.push("/(tabs)/orders") : undefined}
                seeAllLabel={`${data.stats.orders_pending} pending`}
              >
                {pendingOrders.map((order, i) => (
                  <RowWrapper key={order.id} divider={i > 0}>
                    <DashboardOrderRow
                      order={order}
                      currencyCode={currencyCode}
                      onPress={() => router.push(`/(tabs)/orders/${order.id}`)}
                    />
                  </RowWrapper>
                ))}
              </Section>
            ) : null}

            {data.stats.pending_reviews > 0 ? (
              <Section title="Needs attention">
                <ListRow
                  primary="Reviews to moderate"
                  secondary="Awaiting your approval"
                  trailing={String(data.stats.pending_reviews)}
                  trailingTone="accent"
                  onPress={() => router.push("/(tabs)/customers/reviews")}
                  accessibilityLabel={`${data.stats.pending_reviews} reviews awaiting moderation`}
                />
              </Section>
            ) : null}

            {otherRecent.length > 0 ? (
              <Section
                title="Recent Orders"
                onSeeAll={() => router.push("/(tabs)/orders")}
              >
                {otherRecent.map((order, i) => (
                  <RowWrapper key={order.id} divider={i > 0}>
                    <DashboardOrderRow
                      order={order}
                      currencyCode={currencyCode}
                      onPress={() => router.push(`/(tabs)/orders/${order.id}`)}
                    />
                  </RowWrapper>
                ))}
              </Section>
            ) : null}

            {data.low_stock.length > 0 ? (
              <Section title="Low Stock" onSeeAll={() => router.push("/(tabs)/products")}>
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
                    <TopProductRow
                      product={p}
                      rank={i + 1}
                      currencyCode={currencyCode}
                      onPress={() => router.push(`/(tabs)/products/${p.id}`)}
                    />
                  </RowWrapper>
                ))}
              </Section>
            ) : null}

            {setupDone < CHECKLIST_ITEMS ? (
              <Section title="Finish setup">
                <ListRow
                  primary="Complete your store setup"
                  secondary={`${setupDone} of ${CHECKLIST_ITEMS} steps done`}
                  trailing={`${Math.round((setupDone / CHECKLIST_ITEMS) * 100)}%`}
                  trailingTone="accent"
                  onPress={() => router.push("/(tabs)/more/settings")}
                  accessibilityLabel={`Store setup ${setupDone} of ${CHECKLIST_ITEMS} steps complete`}
                />
              </Section>
            ) : null}
          </View>
        )}
      </ScrollView>
    </Screen>
  );
}

function LowStockRow({ item, onPress }: { item: LowStockItem; onPress: () => void }) {
  const label = item.variant_title ? `${item.title} · ${item.variant_title}` : item.title;
  return (
    <ListRow
      primary={label}
      trailing={`${item.quantity} left`}
      trailingTone="danger"
      onPress={onPress}
      accessibilityLabel={`${label}, ${item.quantity} left in stock`}
    />
  );
}

function TopProductRow({
  product,
  rank,
  currencyCode,
  onPress,
}: {
  product: TopProduct;
  rank: number;
  currencyCode?: string;
  onPress: () => void;
}) {
  return (
    <ListRow
      primary={product.title}
      secondary={`${product.units_sold} sold`}
      trailing={formatMoney(product.revenue, currencyCode)}
      leading={
        <Text preset="h3" color="textTertiary" style={styles.rank}>
          {String(rank).padStart(2, "0")}
        </Text>
      }
      onPress={onPress}
      accessibilityLabel={`Number ${rank}, ${product.title}, ${product.units_sold} sold, ${formatMoney(product.revenue, currencyCode)} revenue`}
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
  leading: { width: 28, alignItems: "flex-start" },
  rank: { fontVariant: ["tabular-nums"] },
  chevron: { marginLeft: -theme.spacing.xs },
});

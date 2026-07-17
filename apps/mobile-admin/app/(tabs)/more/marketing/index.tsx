import { ScrollView, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { Ticket } from "lucide-react-native";
import { BackHeader, Screen } from "@/components/ui";
import { MarketingRow } from "@/components/marketing/MarketingRow";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

const ICON_SIZE = 20;
const ICON_STROKE = 1.75;

export default function MarketingHubScreen() {
  const router = useRouter();
  const dockPad = useDockClearance();

  return (
    <Screen>
      <BackHeader eyebrow="MORE" title="Marketing" />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        <MarketingRow
          icon={<Ticket size={ICON_SIZE} color={theme.colors.text} strokeWidth={ICON_STROKE} />}
          label="Coupons"
          description="Discount codes for your store"
          onPress={() => router.push("/(tabs)/more/marketing/coupons")}
        />
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingTop: theme.spacing.sm },
});

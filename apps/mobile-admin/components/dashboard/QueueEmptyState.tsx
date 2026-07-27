import { View, StyleSheet } from "react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";

/**
 * The empty "needs you" queue — an editorial moment, not blank paper and
 * not an apology. Deliberately distinct from both the loading state (a muted
 * caption) and the source-error row (which names what broke): reaching an
 * empty queue is the merchant WINNING, and the serif is what says so.
 *
 * Left-aligned on the screen gutter like every other block on this screen —
 * a centred hero would be the one thing on the Dashboard that isn't.
 */
export function QueueEmptyState() {
  return (
    <View style={styles.root} testID="dashboard-all-clear">
      <Text preset="h1" color="text">
        All clear
      </Text>
      <Text preset="body" color="textSecondary" style={styles.body}>
        No orders waiting, no reviews to moderate, nothing running low. Enjoy
        the quiet.
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  root: {
    paddingHorizontal: theme.spacing.xl,
    paddingTop: theme.spacing.sm,
    paddingBottom: theme.spacing.xxl,
  },
  body: { marginTop: theme.spacing.xs, maxWidth: 320 },
});

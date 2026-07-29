import type { ReactNode } from "react";
import { StyleSheet, View } from "react-native";
import { Card } from "./Card";
import { Hairline } from "./Hairline";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

export interface GroupedListSection {
  key: string;
  /** Rendered as an eyebrow above the card. Omit for an unlabelled group. */
  label?: string;
  rows: ReactNode[];
  /** Explanatory caption below the card, e.g. notification-settings' intro copy. */
  footer?: string;
}

export interface GroupedListProps {
  sections: GroupedListSection[];
}

// 52pt — theme.spacing.huge (48) + theme.spacing.xs (4). Aligns the hairline
// under the label column, past the 22pt icon slot + its row gap.
const HAIRLINE_INSET = theme.spacing.huge + theme.spacing.xs;

/**
 * The grouped-inset-list construction promoted out of `more/index.tsx`'s
 * hand-built section loop, unchanged: an eyebrow above an outlined Card,
 * whose rows are joined by inset hairlines (none leading, none trailing),
 * with an optional caption below the card. Sections are spaced
 * `theme.spacing.lg` apart — the same gap `more/index.tsx`'s ScrollView body
 * already applied between its section Views, now owned by this primitive so
 * every other screen that renders more than one `GroupedList` block gets it
 * for free too.
 */
export function GroupedList({ sections }: GroupedListProps) {
  return (
    <View style={styles.wrap}>
      {sections.map((section) => (
        <View key={section.key} style={styles.section}>
          {section.label ? (
            <Text preset="eyebrow" color="textTertiary" style={styles.sectionLabel}>
              {section.label}
            </Text>
          ) : null}
          <Card padding={0}>
            {section.rows.map((row, i) => (
              // eslint-disable-next-line react/no-array-index-key -- rows are
              // opaque ReactNodes; the caller owns any real key on each one.
              <View key={i}>
                {i > 0 ? <Hairline inset={HAIRLINE_INSET} /> : null}
                {row}
              </View>
            ))}
          </Card>
          {section.footer ? (
            <Text preset="caption" color="textTertiary" style={styles.footer}>
              {section.footer}
            </Text>
          ) : null}
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { gap: theme.spacing.lg },
  section: { gap: theme.spacing.sm },
  sectionLabel: { paddingHorizontal: theme.spacing.xs },
  footer: { paddingHorizontal: theme.spacing.xs },
});

import { type ReactNode } from "react";
import { StyleSheet, View, useWindowDimensions, type AccessibilityRole } from "react-native";
import { ChevronRight } from "lucide-react-native";
import { PressableRow } from "./PressableRow";
import { SHEET_ACTIONS_STACK_SCALE } from "./SheetActions";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

/**
 * Above this device font scale, a right-hand `value` stacks beneath the
 * label instead of sharing its line with it. Reuses `SheetActions`'
 * `SHEET_ACTIONS_STACK_SCALE` rather than re-declaring the same number: both
 * thresholds exist for the identical reason (`bodyEmphasis` outgrowing a
 * shared line at a realistic device width — see
 * `components/ui/SheetActions.tsx`), so importing the constant means the two
 * can't silently desync. 1.3 is roughly where `bodyEmphasis` reaches ~21pt,
 * and a label like "Notification settings" alongside a value no longer both
 * fit one line at a comfortable row width — either one truncates or the row
 * silently clips, which is the exact "Row labels … clipped before" shape the
 * controller addendum calls out at Step 6.6.
 */
const VALUE_STACK_SCALE = SHEET_ACTIONS_STACK_SCALE;

export interface GroupedRowProps {
  label: string;
  /** lucide glyph, 18px / strokeWidth 1.75, in a 22pt slot. */
  icon?: ReactNode;
  /** Right-hand caption, e.g. account.tsx's field values. */
  value?: string;
  /** Right-hand control, e.g. a Switch. Mutually exclusive with `value`. */
  trailing?: ReactNode;
  /** Crimson count pill, e.g. more/index.tsx's unread badge. */
  badge?: string;
  /**
   * Omit for a NON-INTERACTIVE information row. That renders a plain View
   * with identical metrics — NOT a disabled PressableRow, which announces as
   * a disabled button and is wrong for a value the merchant is only reading.
   */
  onPress?: () => void;
  accessibilityLabel?: string;
  accessibilityRole?: AccessibilityRole;
  /** Trailing chevron. Defaults to `true` when `onPress` is set. */
  chevron?: boolean;
  testID?: string;
  /**
   * Secondary descriptive caption rendered BELOW the label, unconditionally
   * (not gated on font scale the way a stacked `value` is) — e.g. a
   * notification preference's explanation, or a store's slug under its name.
   *
   * ADDITIVE beyond the task-9 brief's original interface block: without it
   * there is no way to carry `notification-settings.tsx`'s per-type hint
   * copy or the push-toggle's device description, both of which the
   * per-screen migration table requires preserving. Dropping that copy
   * silently to fit the literal interface would have been an unannounced
   * information-loss regression, not a faithful extraction — see the task-9
   * report for the full justification.
   */
  hint?: string;
  /**
   * Forces the label-above/value-below stacked layout — with the value free
   * to wrap onto multiple lines — REGARDLESS of `fontScale`, instead of only
   * above `VALUE_STACK_SCALE`. Below that threshold `value` otherwise renders
   * inline with `numberOfLines={1}`, which is fine for a short, bounded
   * caption but silently clips an unbounded one (an email address, a full
   * name) at ordinary text size on an ordinary phone width. `jane@example.com`
   * (16 chars) never reproduced this; `firstname.lastname@theircompany.com.au`
   * does, at 100% text size, on a 390pt-wide device.
   *
   * ADDITIVE opt-in: every other `GroupedRow` call site keeps the existing
   * inline-with-clamp behaviour unless it sets this explicitly. Use it for
   * values that are identity data the merchant must be able to read in full
   * (their own email, their own name) rather than a short enum/status
   * caption a single clamped line was always going to be enough for.
   */
  wrapValue?: boolean;
}

/**
 * The grouped-inset-list row primitive — `PressableRow lines={1}` (64pt)
 * promoted out of `more/index.tsx`'s local `Row`, with the non-interactive
 * "value the merchant is only reading" variant `more/index.tsx` never
 * needed added alongside it.
 */
export function GroupedRow({
  label,
  icon,
  value,
  trailing,
  badge,
  onPress,
  accessibilityLabel,
  accessibilityRole,
  chevron,
  testID,
  hint,
  wrapValue,
}: GroupedRowProps) {
  const { fontScale } = useWindowDimensions();
  const stackValue = value !== undefined && (wrapValue || fontScale > VALUE_STACK_SCALE);
  const showChevron = chevron ?? Boolean(onPress);
  const computedLabel = accessibilityLabel ?? (value ? `${label}, ${value}` : label);

  const content = (
    <>
      {icon ? <View style={styles.icon}>{icon}</View> : null}
      <View style={styles.textBlock}>
        {/* No `numberOfLines`: the old local `Row` this was promoted from
            never clamped its label either, letting it wrap and the row (a
            `minHeight`, never a `height`) grow to fit. That's the mechanism
            that lets this row survive accessibility text sizes — clamping
            here would silently ellipsize labels like "Notification
            settings" at 2x instead. See the controller addendum's Step 6.6
            and the code-review finding this fixes. */}
        <Text preset="bodyEmphasis" color="text">
          {label}
        </Text>
        {hint ? (
          <Text preset="caption" color="textTertiary" style={styles.secondaryLine}>
            {hint}
          </Text>
        ) : null}
        {stackValue ? (
          <Text
            preset="caption"
            color="textTertiary"
            style={styles.secondaryLine}
            testID={testID ? `${testID}-value-stacked` : undefined}
          >
            {value}
          </Text>
        ) : null}
      </View>
      {value !== undefined && !stackValue ? (
        <Text
          preset="caption"
          color="textTertiary"
          numberOfLines={1}
          style={styles.value}
          testID={testID ? `${testID}-value-inline` : undefined}
        >
          {value}
        </Text>
      ) : null}
      {trailing ? <View style={styles.trailing}>{trailing}</View> : null}
      {badge ? (
        <View style={styles.badge}>
          <Text preset="caption" color="inverse" style={styles.badgeLabel}>
            {badge}
          </Text>
        </View>
      ) : null}
      {showChevron ? (
        // Wrapped rather than putting `testID` on `ChevronRight` itself: the
        // lucide-react-native jest stub every icon-consuming test in this
        // app uses (`new Proxy({}, { get: () => () => null })`) discards all
        // props including `testID`, so a query against the icon directly
        // would never find it under test even though it renders in the app.
        <View testID={testID ? `${testID}-chevron` : undefined}>
          <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
        </View>
      ) : null}
    </>
  );

  if (!onPress) {
    return (
      <View
        style={styles.nonInteractiveRow}
        testID={testID}
        accessible
        accessibilityLabel={computedLabel}
      >
        {content}
      </View>
    );
  }

  return (
    <PressableRow
      style={styles.row}
      onPress={onPress}
      lines={1}
      accessibilityLabel={computedLabel}
      accessibilityRole={accessibilityRole}
      testID={testID}
    >
      {content}
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  // Pre-migration these rows had no backgroundColor of their own
  // (transparent), letting the parent Card's elevated (white) surface show
  // through. PressableRow's base sets backgroundColor: theme.colors.background
  // (paper), which would otherwise paint a visible seam against the Card —
  // match that surface explicitly instead of relying on transparency (same
  // fix as more/index.tsx's Row and StorePicker).
  row: { backgroundColor: theme.colors.elevated },
  // The non-interactive branch doesn't go through PressableRow, so its
  // layout metrics are spelled out explicitly here. They MUST match
  // PressableRow's own `base` + `oneLine` styles exactly (same theme.row.*
  // tokens) — "identical metrics" per the controller addendum — and MUST use
  // `minHeight`, never `height`: a fixed height holding scalable text is
  // this app's single most repeated defect.
  nonInteractiveRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.row.gap,
    paddingHorizontal: theme.row.paddingH,
    paddingVertical: theme.row.paddingV,
    minHeight: theme.row.minHeightSingle,
    backgroundColor: theme.colors.elevated,
  },
  // A glyph doesn't scale with text, so this box may stay a fixed width —
  // but it carries no `height`, so it never constrains the row's own
  // (unbounded) minHeight-only floor.
  icon: { width: 22, alignItems: "center" },
  textBlock: { flex: 1 },
  secondaryLine: { marginTop: 2 },
  value: { flexShrink: 1, marginLeft: theme.spacing.xs },
  trailing: {},
  badge: {
    backgroundColor: theme.colors.danger,
    borderRadius: 10,
    minWidth: 22,
    height: 20,
    paddingHorizontal: theme.spacing.xs,
    alignItems: "center",
    justifyContent: "center",
    // The old local `Row` wrapped its trailing node (the badge) in
    // `rowTrailing` (`marginRight: theme.spacing.xs`) on top of
    // `PressableRow`'s own `gap: theme.row.gap` (16) — 20pt total between
    // badge and chevron. `badge` here is rendered as its own sibling
    // (not through `trailing`), so that 4pt has to be restored explicitly
    // or the badge sits 16pt from the chevron instead of 20pt.
    marginRight: theme.spacing.xs,
  },
  badgeLabel: { fontSize: 10, fontWeight: "700" },
});

import { View, StyleSheet } from "react-native";
import { Text, type TextPreset } from "./Text";
import { theme } from "@/lib/theme";

/** First letter of `label`, or `?` when there is nothing to take one from. */
export function monogramInitial(label: string): string {
  const trimmed = label.trim();
  return trimmed ? trimmed.charAt(0).toUpperCase() : "?";
}

export interface MonogramProps {
  /** The name (or email) whose first letter is drawn. */
  label: string;
  /** Box edge in unscaled points. Defaults to `Thumb`'s 60pt list size. */
  size?: number;
  /** Typography for the initial. Defaults to the 17pt row preset. */
  textPreset?: TextPreset;
  testID?: string;
}

/**
 * The leading-art tile for a person: their initial on a neutral field, drawn
 * wherever a customer has no photo — Dashboard queue rows, the customers list,
 * the customer detail head.
 *
 * PROMOTED from `QueueRow`, which said in as many words: "Promote this to
 * components/ui if a second caller needs the same tile." Three callers now do,
 * and they had drifted into three different renderings of the same element —
 * the queue a 60pt rounded square, the customers list a 40pt circle, the
 * customer detail a 72pt circle. Same element, three screens, two shapes.
 *
 * ROUNDED SQUARE, at `Thumb`'s radius — the direction is settled by evidence,
 * not taste:
 *
 *  - `Thumb` (components/ui/Thumb.tsx) is this app's image primitive and is a
 *    rounded square. It is the leading-art shape every product/order list
 *    already draws, and it is NOT the thing that moves (it has far more call
 *    sites, and its own placeholder must match a real photo, which is
 *    rectangular).
 *  - The Dashboard queue's monogram was deliberately aligned to `Thumb` for
 *    exactly this reason: an order WITH a product photo and one WITHOUT must
 *    occupy the same slot and read as one column. That fix scoped itself to
 *    the queue; this completes it.
 *  - So the shape that unifies is the one that already has to hold an image.
 *    A circle cannot; a rounded square can.
 *
 * Fill is `sink` with a hairline `textTertiary` ring, NOT `surfaceAlt` — both
 * choices are load-bearing and both were measured on device:
 *
 *  - `surfaceAlt` (#FAF8F2) is all but invisible against Paper (#F7F6F2), and
 *    the detail head sits directly on Paper.
 *  - `PressableRow`'s iOS pressed state repaints the row background to `sink`,
 *    so a fill-only tile has zero contrast against its own row while held.
 *    `Thumb`'s placeholder and the queue monogram both already carry this ring
 *    for that reason; drawing the customers list from the same component is
 *    what makes the three tiles actually identical rather than approximately
 *    so.
 *
 * Never moss. The monogram is decoration on an identity, and this app's one
 * accent is spent elsewhere on every screen that draws one.
 */
export function Monogram({
  label,
  size = theme.thumb.list,
  textPreset = "bodyEmphasis",
  testID,
}: MonogramProps) {
  return (
    <View
      style={[styles.box, { width: size, height: size, borderRadius: theme.radii.md }]}
      accessible={false}
      testID={testID}
    >
      <Text
        preset={textPreset}
        color="text"
        // The box is a FIXED square, so a scaled line box has nowhere to go:
        // at 2x, `bodyEmphasis`'s 24pt line becomes 48pt inside the 40pt tile
        // the customers list draws, and the glyph is sliced through the stem.
        // Same ruling `TenantMonogram` already made for the same reason — a
        // monogram is an identity MARK, not content: WCAG 2.1 SC 1.4.4 has
        // nothing to resize here, the letter is `accessible={false}`, and the
        // name it is the first letter of sits beside it at full Dynamic Type.
        maxFontSizeMultiplier={1}
        // Pin the line box to the tile so the glyph is optically centred at
        // every `size`/`textPreset` pair instead of riding high or low on the
        // preset's own lineHeight (the trick TenantMonogram's disc uses).
        style={{ lineHeight: size, includeFontPadding: false }}
      >
        {monogramInitial(label)}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  box: {
    backgroundColor: theme.colors.sink,
    borderWidth: theme.hairline,
    borderColor: theme.colors.textTertiary,
    alignItems: "center",
    justifyContent: "center",
    flexShrink: 0,
    overflow: "hidden",
  },
});

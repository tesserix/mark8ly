import { useState } from "react";
import { View, Image, TextInput, Pressable, ScrollView, StyleSheet } from "react-native";
import { ChevronLeft, ChevronRight } from "lucide-react-native";
import { IconButton, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { ProductMedia } from "@repo/mobile-shared/api/schemas/products";

/** The wire does not guarantee array order — order by position. Pure; no mutation. */
export function sortMedia(media: ProductMedia[]): ProductMedia[] {
  return [...media].sort((a, b) => a.position - b.position);
}

/** One row of the reorder swap: which media id moves to which position. */
export interface MediaReorderWrite {
  id: string;
  position: number;
}

/**
 * The two writes needed to move a photo to `newPosition`.
 *
 * 🔴 A single-row position PATCH CANNOT reorder a list. The backend
 * `UpdateMedia` (service_single_media.go) writes only the row it is handed and
 * does NOT shift the sibling already sitting at that position — so a one-row
 * write leaves two photos sharing a position, and the storefront hero (position
 * 0) flips between refetches. The reorder UI is adjacent move-earlier /
 * move-later, so the correct operation is a SWAP: give the moved item the
 * target slot and hand its old slot to whoever held it. Two PATCHes, never one.
 *
 * Returns `[]` when there is nothing to swap (unknown id, empty target slot, or
 * a no-op onto its own position) so the two writes can never collide or
 * duplicate a position. Pure; find-by-position, so array order does not matter.
 */
export function computeReorderWrites(
  media: ProductMedia[],
  movedId: string,
  newPosition: number,
): MediaReorderWrite[] {
  const moved = media.find((m) => m.id === movedId);
  const displaced = media.find((m) => m.position === newPosition);
  if (!moved || !displaced || moved.id === displaced.id) return [];
  return [
    { id: moved.id, position: newPosition },
    { id: displaced.id, position: moved.position },
  ];
}

interface MediaGridProps {
  media: ProductMedia[];
  onReorder: (mediaId: string, position: number) => void;
  onAltChange: (mediaId: string, alt: string) => void;
  onPress: (media: ProductMedia) => void;
  onLongPress: (mediaId: string) => void;
}

interface AltInputProps {
  media: ProductMedia;
  index: number;
  onAltChange: (mediaId: string, alt: string) => void;
}

function AltInput({ media, index, onAltChange }: AltInputProps) {
  const [alt, setAlt] = useState(media.alt ?? "");
  const handleBlur = () => {
    const trimmed = alt.trim();
    if (trimmed === (media.alt ?? "")) return;
    onAltChange(media.id, trimmed);
  };
  return (
    <TextInput
      style={styles.alt}
      value={alt}
      onChangeText={setAlt}
      onBlur={handleBlur}
      placeholder="Alt text"
      placeholderTextColor={theme.colors.textTertiary}
      accessibilityLabel={`Alt text for photo ${index + 1}`}
    />
  );
}

/**
 * Reorder is move-earlier/move-later, not drag-and-drop: @dnd-kit (what the web
 * admin uses) is web-only, and adding an RN drag library would mean an npm
 * install, which this repo forbids. Buttons are also more accessible.
 */
export function MediaGrid({ media, onReorder, onAltChange, onPress, onLongPress }: MediaGridProps) {
  const ordered = sortMedia(media);

  return (
    <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.root}>
      {ordered.map((m, i) => (
        <View key={m.id} style={styles.item}>
          <Pressable
            onPress={() => onPress(m)}
            onLongPress={() => onLongPress(m.id)}
            accessibilityRole="imagebutton"
            accessibilityLabel={i === 0 ? `Photo 1, main image` : `Photo ${i + 1}`}
          >
            <Image source={{ uri: m.url }} style={styles.thumb} />
          </Pressable>

          <View style={styles.controls}>
            {i > 0 ? (
              <IconButton
                onPress={() => onReorder(m.id, i - 1)}
                accessibilityLabel={`Move photo ${i + 1} earlier`}
              >
                <ChevronLeft size={16} color={theme.colors.text} strokeWidth={2} />
              </IconButton>
            ) : null}
            {i === 0 ? (
              <Text preset="caption" color="textTertiary">
                Main
              </Text>
            ) : null}
            {i < ordered.length - 1 ? (
              <IconButton
                onPress={() => onReorder(m.id, i + 1)}
                accessibilityLabel={`Move photo ${i + 1} later`}
              >
                <ChevronRight size={16} color={theme.colors.text} strokeWidth={2} />
              </IconButton>
            ) : null}
          </View>

          <AltInput media={m} index={i} onAltChange={onAltChange} />
        </View>
      ))}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  root: { gap: theme.spacing.sm, paddingVertical: theme.spacing.xs },
  item: { gap: theme.spacing.xs, width: 120 },
  thumb: {
    width: 120,
    height: 120,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
  },
  // Was a fixed `height: 24` sized to the bare 16px chevron glyph. The
  // reorder buttons are now IconButton, whose real 44pt touch target (not
  // hitSlop) would overflow a fixed 24pt row and sit on top of `alt` below
  // it — `minHeight` lets the row grow to fit instead.
  controls: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    minHeight: theme.touchTarget,
  },
  alt: {
    height: 36,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.sm,
    paddingHorizontal: theme.spacing.xs,
    // Was a literal 12 (the old caption scale) — orphaned by the type
    // rescale. theme.text.caption.fontSize stays anchored to the current
    // scale.
    fontSize: theme.text.caption.fontSize,
    color: theme.colors.text,
    backgroundColor: theme.colors.elevated,
  },
});

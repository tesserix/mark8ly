import { useState } from "react";
import { View, Image, TextInput, Pressable, ScrollView, StyleSheet } from "react-native";
import { ChevronLeft, ChevronRight } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { ProductMedia } from "@repo/mobile-shared/api/schemas/products";

/** The wire does not guarantee array order — order by position. Pure; no mutation. */
export function sortMedia(media: ProductMedia[]): ProductMedia[] {
  return [...media].sort((a, b) => a.position - b.position);
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
              <Pressable
                onPress={() => onReorder(m.id, i - 1)}
                accessibilityRole="button"
                accessibilityLabel={`Move photo ${i + 1} earlier`}
                hitSlop={8}
              >
                <ChevronLeft size={16} color={theme.colors.text} strokeWidth={2} />
              </Pressable>
            ) : null}
            {i === 0 ? (
              <Text preset="caption" color="textTertiary">
                Main
              </Text>
            ) : null}
            {i < ordered.length - 1 ? (
              <Pressable
                onPress={() => onReorder(m.id, i + 1)}
                accessibilityRole="button"
                accessibilityLabel={`Move photo ${i + 1} later`}
                hitSlop={8}
              >
                <ChevronRight size={16} color={theme.colors.text} strokeWidth={2} />
              </Pressable>
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
  controls: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    height: 24,
  },
  alt: {
    height: 36,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.sm,
    paddingHorizontal: theme.spacing.xs,
    fontSize: 12,
    color: theme.colors.text,
    backgroundColor: theme.colors.elevated,
  },
});

import { useState } from "react";
import { View, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { Package } from "lucide-react-native";
import { theme } from "@/lib/theme";

/**
 * List thumbnail on expo-image (already a dependency, previously used only
 * in ProductMediaPicker). react-native's `Image` pops in with no transition,
 * which reads as a web image load.
 *
 * A missing, null, or failed `uri` renders the placeholder at the SAME
 * dimensions, so a row never changes height because an image 404'd.
 */
export interface ThumbProps {
  uri?: string | null;
  /** "list" = 60pt (default), "compact" = 38pt. */
  size?: "list" | "compact";
  /** Stable item id — stops FlatList recycling flashing a stale image. */
  recyclingKey?: string;
  accessibilityLabel?: string;
  testID?: string;
}

export function Thumb({
  uri,
  size = "list",
  recyclingKey,
  accessibilityLabel,
  testID,
}: ThumbProps) {
  const dim = size === "list" ? theme.thumb.list : theme.thumb.compact;
  const box = { width: dim, height: dim };

  // Track load failure so a 404'd image falls back to the placeholder
  // instead of a broken-image box. Reset during render (not in an effect)
  // when `uri` changes from the one we last saw a failure for — a
  // FlatList-recycled row that failed once must not show the placeholder
  // forever for every subsequent item that reuses this component instance.
  const [failed, setFailed] = useState(false);
  const [lastUri, setLastUri] = useState(uri);
  if (uri !== lastUri) {
    setLastUri(uri);
    setFailed(false);
  }

  if (!uri || failed) {
    return (
      <View
        style={[styles.box, box, styles.placeholder]}
        accessible={false}
        testID={testID}
      >
        <Package
          size={Math.round(dim / 3)}
          color={theme.colors.textTertiary}
          strokeWidth={1.5}
        />
      </View>
    );
  }

  return (
    <Image
      source={{ uri }}
      style={[styles.box, box]}
      contentFit="cover"
      transition={200}
      recyclingKey={recyclingKey}
      accessible={false}
      accessibilityLabel={accessibilityLabel}
      testID={testID}
      onError={() => setFailed(true)}
    />
  );
}

const styles = StyleSheet.create({
  box: {
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.sink,
    flexShrink: 0,
  },
  placeholder: {
    alignItems: "center",
    justifyContent: "center",
  },
});

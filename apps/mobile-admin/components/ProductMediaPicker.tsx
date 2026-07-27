import { useState } from "react";
import { Platform, View, Image, Pressable, ScrollView, StyleSheet } from "react-native";
import * as ImagePicker from "expo-image-picker";
import { Camera, Image as ImageIcon, X } from "lucide-react-native";
import { IconButton, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

interface ProductMediaPickerProps {
  images: string[];
  onImagesChange: (uris: string[]) => void;
}

// Expanded hit region for the remove badge, computed against the real
// geometry (see the `removeBtn` style comment below for the full
// derivation). Right/bottom stay tight because the next thumbnail sits
// only `gallery`'s 8pt gap away; top/left have no neighbour to clip so they
// carry the rest of the way toward a 44pt target.
const REMOVE_BTN_HIT_SLOP = { top: 12, left: 21, bottom: 10, right: 1 };

/**
 * The geometry the no-overlap proof depends on, exported so a test can assert
 * the invariant instead of trusting the comment above.
 *
 * The failure mode here is destructive — if the badge's hit region reaches the
 * next thumbnail, tapping near image N+1 deletes image N. That regression
 * would otherwise be silent: no type error, no failing test. See
 * `__tests__/media-picker-hit-region.test.ts`.
 */
export const REMOVE_BTN_GEOMETRY = {
  thumbSize: 80,
  badgeSize: 22,
  /** Badge is offset outward from the thumb's top-right corner by this much. */
  badgeOffset: 6,
  hitSlop: REMOVE_BTN_HIT_SLOP,
  /** Horizontal gap between adjacent thumbnails — `gallery`'s gap. */
  siblingGap: theme.spacing.sm,
} as const;

function RemoveImageButton({ index, onRemove }: { index: number; onRemove: (index: number) => void }) {
  return (
    <IconButton
      onPress={() => onRemove(index)}
      accessibilityLabel={`Remove image ${index + 1}`}
      tone="onDark"
      style={styles.removeBtn}
      hitSlop={REMOVE_BTN_HIT_SLOP}
    >
      <X size={12} color={theme.colors.inverse} strokeWidth={2.5} />
    </IconButton>
  );
}

export function ProductMediaPicker({ images, onImagesChange }: ProductMediaPickerProps) {
  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [cameraPressed, setCameraPressed] = useState(false);
  const [libraryPressed, setLibraryPressed] = useState(false);

  const takePhoto = async () => {
    const result = await ImagePicker.launchCameraAsync({
      mediaTypes: ["images"],
      quality: 0.8,
      allowsEditing: true,
    });
    if (!result.canceled && result.assets[0]) {
      onImagesChange([...images, result.assets[0].uri]);
    }
  };

  const pickFromLibrary = async () => {
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ["images"],
      quality: 0.8,
      allowsMultipleSelection: true,
      selectionLimit: 10,
    });
    if (!result.canceled) {
      const newUris = result.assets.map((a) => a.uri);
      onImagesChange([...images, ...newUris]);
    }
  };

  const removeImage = (index: number) => {
    onImagesChange(images.filter((_, i) => i !== index));
  };

  return (
    <View style={styles.container}>
      <View style={styles.buttons}>
        <Pressable
          onPress={takePhoto}
          onPressIn={() => setCameraPressed(true)}
          onPressOut={() => setCameraPressed(false)}
          accessibilityRole="button"
          accessibilityLabel="Take photo"
          android_ripple={theme.press.rippleInk}
          style={[
            styles.button,
            cameraPressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
          ]}
        >
          <Camera size={16} color={theme.colors.text} strokeWidth={1.75} />
          <Text preset="bodyEmphasis" color="text">
            Camera
          </Text>
        </Pressable>
        <Pressable
          onPress={pickFromLibrary}
          onPressIn={() => setLibraryPressed(true)}
          onPressOut={() => setLibraryPressed(false)}
          accessibilityRole="button"
          accessibilityLabel="Choose from library"
          android_ripple={theme.press.rippleInk}
          style={[
            styles.button,
            libraryPressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
          ]}
        >
          <ImageIcon size={16} color={theme.colors.text} strokeWidth={1.75} />
          <Text preset="bodyEmphasis" color="text">
            Library
          </Text>
        </Pressable>
      </View>
      {images.length > 0 ? (
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={styles.gallery}
        >
          {images.map((uri, i) => (
            <View key={`${uri}-${i}`} style={styles.thumbWrap}>
              <Image
                source={{ uri }}
                style={styles.thumb}
                accessibilityLabel={`Selected image ${i + 1}`}
              />
              <RemoveImageButton index={i} onRemove={removeImage} />
            </View>
          ))}
        </ScrollView>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: theme.spacing.md },
  buttons: { flexDirection: "row", gap: theme.spacing.sm },
  button: {
    flex: 1,
    flexDirection: "row",
    gap: theme.spacing.xs,
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    height: 44,
    alignItems: "center",
    justifyContent: "center",
  },
  gallery: { gap: theme.spacing.sm, paddingVertical: theme.spacing.xs },
  // Width/height are explicit rather than auto-sized from the image child.
  // The remove badge's no-overlap clearance is derived from this being
  // exactly 80pt; if a future in-flow child (a caption, a progress label)
  // changed the auto-size, the clearance proof would break silently.
  thumbWrap: {
    position: "relative",
    width: REMOVE_BTN_GEOMETRY.thumbSize,
    height: REMOVE_BTN_GEOMETRY.thumbSize,
  },
  thumb: {
    width: 80,
    height: 80,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
  },
  // Overlay badge, not a standalone icon button — it sits ON the thumbnail,
  // and the next thumbnail starts only `gallery`'s 8pt gap away. A real 44pt
  // IconButton box here would both (a) cover over half the 80pt thumbnail
  // and (b) hang ~6pt of invisible hit area onto the neighbouring photo, so
  // a tap meant for image N+1 could fire "remove image N". Restored to the
  // pre-migration 22px visible circle at top/right: -6 and paired with
  // IconButton's `hitSlop` escape hatch (REMOVE_BTN_HIT_SLOP above) instead
  // of a real box. Geometry (thumbWrap-local coords, x right / y down):
  //   badge box:      x [64, 86], y [-6, 16]        (22×22 at top/right:-6)
  //   next thumbWrap: x [88, 168]                   (80pt width + 8pt gap)
  //   hit region:     x [43, 87], y [-18, 26]        (badge box + hitSlop)
  // Right edge of the hit region (87) stays 1pt inside the next thumbnail's
  // left edge (88) — see REMOVE_BTN_HIT_SLOP's comment and the task report
  // for the full arithmetic. Do not widen `right`/`bottom` past this without
  // re-deriving the clearance.
  removeBtn: {
    position: "absolute",
    top: -6,
    right: -6,
    width: 22,
    height: 22,
    borderRadius: theme.radii.pill,
    backgroundColor: theme.colors.text,
  },
});

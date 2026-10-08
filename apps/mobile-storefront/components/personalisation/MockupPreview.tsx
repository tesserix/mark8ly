import { useState } from "react";
import { StyleSheet, View } from "react-native";
import { Image } from "expo-image";

import { theme } from "@/lib/theme";

/**
 * The merchant's mockup with the buyer's photo composited into the print
 * area (#966, on mobile for #969).
 *
 * print_area is in PERCENTAGES of the mockup's own dimensions, so the
 * container must have the mockup's aspect ratio or the rectangle lands in
 * the wrong place. The ratio is read off the loaded image rather than
 * assumed square; until it loads the outline is drawn over a square
 * placeholder, which is close enough for a frame.
 *
 * Rendered before the buyer picks too: the outline alone shows WHERE the
 * photo goes, which is most of the value up front.
 */
export function MockupPreview({
  mockupUrl,
  printArea,
  artworkUri,
  fieldLabel,
}: {
  mockupUrl: string;
  printArea: { x: number; y: number; w: number; h: number };
  /** Local file URI of the picked photo. Absent until one is chosen. */
  artworkUri?: string;
  fieldLabel: string;
}) {
  const [ratio, setRatio] = useState(1);
  return (
    <View
      style={[styles.frame, { aspectRatio: ratio }]}
      accessibilityRole="image"
      accessibilityLabel={
        artworkUri
          ? `A preview of how your photo will be printed for ${fieldLabel}`
          : `Where your photo for ${fieldLabel} will be printed`
      }
    >
      <Image
        source={{ uri: mockupUrl }}
        style={StyleSheet.absoluteFill}
        contentFit="contain"
        onLoad={(e) => {
          const { width, height } = e.source;
          if (width > 0 && height > 0) setRatio(width / height);
        }}
        accessibilityIgnoresInvertColors
      />
      <View
        style={[
          styles.area,
          {
            left: `${printArea.x}%`,
            top: `${printArea.y}%`,
            width: `${printArea.w}%`,
            height: `${printArea.h}%`,
          },
          artworkUri ? styles.areaFilled : null,
        ]}
      >
        {artworkUri ? (
          <Image
            source={{ uri: artworkUri }}
            style={StyleSheet.absoluteFill}
            contentFit="cover"
            accessibilityIgnoresInvertColors
          />
        ) : null}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  frame: {
    width: "100%",
    borderRadius: theme.radii.lg,
    overflow: "hidden",
    backgroundColor: theme.colors.surfaceAlt,
  },
  area: {
    position: "absolute",
    borderWidth: 1,
    borderStyle: "dashed",
    borderColor: theme.colors.textTertiary,
    overflow: "hidden",
  },
  areaFilled: { borderWidth: 0 },
});

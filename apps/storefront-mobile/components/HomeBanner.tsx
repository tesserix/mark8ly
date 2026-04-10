import { useMemo } from "react";
import { View, Text, StyleSheet, useWindowDimensions } from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { Image } from "expo-image";

interface HomeBannerProps {
  imageUrl?: string;
  title?: string;
}

const BANNER_HEIGHT = 200;

export function HomeBanner({ imageUrl, title }: HomeBannerProps) {
  const theme = useTheme();
  const { width: screenWidth } = useWindowDimensions();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  if (!imageUrl) return null;

  return (
    <View style={[styles.container, { width: screenWidth }]}>
      <Image
        source={{ uri: imageUrl }}
        style={styles.image}
        contentFit="cover"
        transition={300}
      />
      {title ? (
        <View style={styles.overlay}>
          <Text style={styles.title}>{title}</Text>
        </View>
      ) : null}
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    
    height: BANNER_HEIGHT,
    backgroundColor: theme.border,
  },
  image: {
    width: "100%",
    height: "100%",
  },
  overlay: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: "rgba(14, 14, 12, 0.35)",
    justifyContent: "flex-end",
    padding: 20,
  },
  title: {
    fontSize: 24,
    fontWeight: "700",
    color: theme.elevated,
    fontFamily: "SourceSerif4",
  },
});
}

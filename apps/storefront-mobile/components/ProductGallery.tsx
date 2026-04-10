import { useRef, useState, useCallback, useMemo } from "react";
import {
  View,
  FlatList,
  StyleSheet,
  useWindowDimensions,
  type NativeSyntheticEvent,
  type NativeScrollEvent,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { Image } from "expo-image";
import type { StorefrontProductImage } from "@repo/mobile-shared/api/storefront-types";

interface ProductGalleryProps {
  images: StorefrontProductImage[];
}


export function ProductGallery({ images }: ProductGalleryProps) {
  const theme = useTheme();
  const { width: screenWidth } = useWindowDimensions();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const [activeIndex, setActiveIndex] = useState(0);
  const flatListRef = useRef<FlatList>(null);

  const handleScroll = useCallback(
    (event: NativeSyntheticEvent<NativeScrollEvent>) => {
      const offset = event.nativeEvent.contentOffset.x;
      const index = Math.round(offset / screenWidth);
      setActiveIndex(index);
    },
    [],
  );

  if (images.length === 0) {
    return (
      <View style={[styles.placeholder, { width: screenWidth }]} />
    );
  }

  const renderItem = ({ item }: { item: StorefrontProductImage }) => (
    <View style={[styles.slide, { width: screenWidth }]}>
      <Image
        source={{ uri: item.url }}
        style={styles.image}
        contentFit="cover"
        transition={200}
        accessibilityLabel={item.alt || "Product image"}
      />
    </View>
  );

  return (
    <View style={styles.container}>
      <FlatList
        ref={flatListRef}
        data={images}
        keyExtractor={(item) => item.id}
        renderItem={renderItem}
        horizontal
        pagingEnabled
        showsHorizontalScrollIndicator={false}
        onScroll={handleScroll}
        scrollEventThrottle={16}
      />
      {images.length > 1 && (
        <View style={styles.dots}>
          {images.map((img, idx) => (
            <View
              key={img.id}
              style={[styles.dot, idx === activeIndex && styles.dotActive]}
            />
          ))}
        </View>
      )}
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    backgroundColor: theme.background,
  },
  slide: {
    
    aspectRatio: 1,
  },
  image: {
    width: "100%",
    height: "100%",
  },
  placeholder: {
    
    aspectRatio: 1,
    backgroundColor: theme.border,
  },
  dots: {
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    paddingVertical: 12,
    gap: 6,
  },
  dot: {
    width: 6,
    height: 6,
    borderRadius: 3,
    backgroundColor: theme.border,
  },
  dotActive: {
    backgroundColor: theme.primary,
    width: 8,
    height: 8,
    borderRadius: 4,
  },
});
}

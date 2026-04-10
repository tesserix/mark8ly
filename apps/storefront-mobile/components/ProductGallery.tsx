import { useRef, useState, useCallback } from "react";
import {
  View,
  FlatList,
  StyleSheet,
  Dimensions,
  type NativeSyntheticEvent,
  type NativeScrollEvent,
} from "react-native";
import { Image } from "expo-image";
import type { StorefrontProductImage } from "@repo/mobile-shared/api/storefront-types";

interface ProductGalleryProps {
  images: StorefrontProductImage[];
}

const SCREEN_WIDTH = Dimensions.get("window").width;

export function ProductGallery({ images }: ProductGalleryProps) {
  const [activeIndex, setActiveIndex] = useState(0);
  const flatListRef = useRef<FlatList>(null);

  const handleScroll = useCallback(
    (event: NativeSyntheticEvent<NativeScrollEvent>) => {
      const offset = event.nativeEvent.contentOffset.x;
      const index = Math.round(offset / SCREEN_WIDTH);
      setActiveIndex(index);
    },
    [],
  );

  if (images.length === 0) {
    return (
      <View style={styles.placeholder} />
    );
  }

  const renderItem = ({ item }: { item: StorefrontProductImage }) => (
    <View style={styles.slide}>
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

const styles = StyleSheet.create({
  container: {
    backgroundColor: "#F7F6F2",
  },
  slide: {
    width: SCREEN_WIDTH,
    aspectRatio: 1,
  },
  image: {
    width: "100%",
    height: "100%",
  },
  placeholder: {
    width: SCREEN_WIDTH,
    aspectRatio: 1,
    backgroundColor: "#E5E4DF",
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
    backgroundColor: "#E5E4DF",
  },
  dotActive: {
    backgroundColor: "#0E0E0C",
    width: 8,
    height: 8,
    borderRadius: 4,
  },
});

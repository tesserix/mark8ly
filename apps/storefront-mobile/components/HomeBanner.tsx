import { View, Text, StyleSheet, Dimensions } from "react-native";
import { Image } from "expo-image";

interface HomeBannerProps {
  imageUrl?: string;
  title?: string;
}

const SCREEN_WIDTH = Dimensions.get("window").width;
const BANNER_HEIGHT = 200;

export function HomeBanner({ imageUrl, title }: HomeBannerProps) {
  if (!imageUrl) return null;

  return (
    <View style={styles.container}>
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

const styles = StyleSheet.create({
  container: {
    width: SCREEN_WIDTH,
    height: BANNER_HEIGHT,
    backgroundColor: "#E5E4DF",
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
    color: "#FFFFFF",
    fontFamily: "SourceSerif4",
  },
});

import { Platform, View, Pressable, Image, Modal, StyleSheet } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";

export interface ViewerImage {
  uri: string;
  alt?: string;
}

interface ImageViewerProps {
  image: ViewerImage | null;
  onClose: () => void;
}

/** Full-screen, dismissable viewer for a tapped product image. Extracted from [id].tsx. */
export function ImageViewer({ image, onClose }: ImageViewerProps) {
  const insets = useSafeAreaInsets();

  return (
    <Modal visible={image !== null} transparent animationType="fade" onRequestClose={onClose}>
      <View style={styles.viewerBackdrop}>
        <Pressable
          onPress={onClose}
          hitSlop={12}
          accessibilityRole="button"
          accessibilityLabel="Close image viewer"
          android_ripple={{ color: "rgba(247, 246, 242, 0.24)", borderless: true }}
          style={({ pressed }) => [
            styles.viewerClose,
            { top: insets.top + theme.spacing.md },
            pressed && Platform.OS === "ios" ? { opacity: 0.55 } : null,
          ]}
        >
          <Text preset="bodyEmphasis" color="inverse">
            Close
          </Text>
        </Pressable>
        {image ? (
          <Image
            source={{ uri: image.uri }}
            style={styles.viewerImage}
            resizeMode="contain"
            accessibilityLabel={image.alt ?? "Product image"}
          />
        ) : null}
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  viewerBackdrop: {
    flex: 1,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  viewerClose: {
    position: "absolute",
    right: theme.spacing.lg,
    zIndex: 1,
    paddingHorizontal: theme.spacing.md,
    paddingVertical: theme.spacing.sm,
  },
  viewerImage: {
    width: "100%",
    height: "80%",
  },
});

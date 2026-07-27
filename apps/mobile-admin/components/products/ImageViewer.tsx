import { useState } from "react";
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
  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [pressed, setPressed] = useState(false);

  return (
    <Modal visible={image !== null} transparent animationType="fade" onRequestClose={onClose}>
      <View style={styles.viewerBackdrop}>
        <Pressable
          onPress={onClose}
          onPressIn={() => setPressed(true)}
          onPressOut={() => setPressed(false)}
          hitSlop={12}
          accessibilityRole="button"
          accessibilityLabel="Close image viewer"
          android_ripple={{ ...theme.press.rippleOnDark, borderless: true }}
          style={[
            styles.viewerClose,
            { top: insets.top + theme.spacing.md },
            pressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
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

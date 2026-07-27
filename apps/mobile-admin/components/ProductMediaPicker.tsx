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

function RemoveImageButton({ index, onRemove }: { index: number; onRemove: (index: number) => void }) {
  return (
    <IconButton
      onPress={() => onRemove(index)}
      accessibilityLabel={`Remove image ${index + 1}`}
      tone="onDark"
      style={styles.removeBtn}
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
  thumbWrap: { position: "relative" },
  thumb: {
    width: 80,
    height: 80,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
  },
  // IconButton's real 44pt minimum hit area (not hitSlop) doubles this
  // badge's footprint from the pre-migration 22px circle — repositioned
  // further outside the thumbnail corner (was top/right: -6) so it reads as
  // a floating corner badge rather than sinking into the image. Visible size
  // change; flagged in the migration report for human visual QA.
  removeBtn: {
    position: "absolute",
    top: -14,
    right: -14,
    borderRadius: theme.radii.pill,
    backgroundColor: theme.colors.text,
  },
});

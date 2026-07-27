import { useState } from "react";
import { Platform, View, Image, Pressable, ScrollView, StyleSheet } from "react-native";
import * as ImagePicker from "expo-image-picker";
import { Camera, Image as ImageIcon, X } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";

interface ProductMediaPickerProps {
  images: string[];
  onImagesChange: (uris: string[]) => void;
}

// Extracted so press state can live in `useState`: this button renders once
// per row inside `images.map()`, and hooks can't be called from inside a
// `.map()` callback — each row needs its own component instance instead.
function RemoveImageButton({ index, onRemove }: { index: number; onRemove: (index: number) => void }) {
  const [pressed, setPressed] = useState(false);
  return (
    <Pressable
      onPress={() => onRemove(index)}
      onPressIn={() => setPressed(true)}
      onPressOut={() => setPressed(false)}
      accessibilityRole="button"
      accessibilityLabel={`Remove image ${index + 1}`}
      hitSlop={8}
      android_ripple={{ ...theme.press.rippleOnDark, borderless: true }}
      style={[
        styles.removeBtn,
        pressed && Platform.OS === "ios" ? { opacity: theme.press.opacitySolidFill } : null,
      ]}
    >
      <X size={12} color={theme.colors.inverse} strokeWidth={2.5} />
    </Pressable>
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
  removeBtn: {
    position: "absolute",
    top: -6,
    right: -6,
    width: 22,
    height: 22,
    borderRadius: 11,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
});

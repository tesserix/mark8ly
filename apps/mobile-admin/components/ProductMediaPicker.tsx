import { View, Text, Image, TouchableOpacity, ScrollView, StyleSheet } from "react-native";
import * as ImagePicker from "expo-image-picker";
import { theme } from "@/lib/theme";

interface ProductMediaPickerProps {
  images: string[];
  onImagesChange: (uris: string[]) => void;
}

export function ProductMediaPicker({ images, onImagesChange }: ProductMediaPickerProps) {
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
        <TouchableOpacity
          style={styles.button}
          onPress={takePhoto}
          activeOpacity={0.7}
          accessibilityRole="button"
          accessibilityLabel="Take photo"
        >
          <Text style={styles.buttonText}>Take Photo</Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={styles.button}
          onPress={pickFromLibrary}
          activeOpacity={0.7}
          accessibilityRole="button"
          accessibilityLabel="Choose from library"
        >
          <Text style={styles.buttonText}>Choose from Library</Text>
        </TouchableOpacity>
      </View>
      {images.length > 0 && (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.gallery}>
          {images.map((uri, i) => (
            <View key={`${uri}-${i}`} style={styles.thumbWrap}>
              <Image
                source={{ uri }}
                style={styles.thumb}
                accessibilityLabel={`Selected image ${i + 1}`}
              />
              <TouchableOpacity
                style={styles.removeBtn}
                onPress={() => removeImage(i)}
                accessibilityRole="button"
                accessibilityLabel={`Remove image ${i + 1}`}
              >
                <Text style={styles.removeText}>x</Text>
              </TouchableOpacity>
            </View>
          ))}
        </ScrollView>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: theme.spacing.md },
  buttons: { flexDirection: "row", gap: theme.spacing.sm },
  button: {
    flex: 1,
    backgroundColor: theme.colors.text,
    borderRadius: theme.radius,
    height: 44,
    alignItems: "center",
    justifyContent: "center",
  },
  buttonText: { color: theme.colors.background, fontSize: 14, fontWeight: "600" },
  gallery: { marginTop: theme.spacing.sm },
  thumbWrap: { position: "relative", marginRight: theme.spacing.sm },
  thumb: { width: 80, height: 80, borderRadius: theme.radius },
  removeBtn: {
    position: "absolute",
    top: -6,
    right: -6,
    width: 22,
    height: 22,
    borderRadius: 11,
    backgroundColor: theme.colors.danger,
    alignItems: "center",
    justifyContent: "center",
  },
  removeText: { color: theme.colors.elevated, fontSize: 14, fontWeight: "700", marginTop: -1 },
});

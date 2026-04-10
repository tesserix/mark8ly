import { View, Text, Image, TouchableOpacity, ScrollView, StyleSheet } from "react-native";
import * as ImagePicker from "expo-image-picker";

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
        <TouchableOpacity style={styles.button} onPress={takePhoto} activeOpacity={0.7}>
          <Text style={styles.buttonText}>Take Photo</Text>
        </TouchableOpacity>
        <TouchableOpacity style={styles.button} onPress={pickFromLibrary} activeOpacity={0.7}>
          <Text style={styles.buttonText}>Choose from Library</Text>
        </TouchableOpacity>
      </View>
      {images.length > 0 && (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.gallery}>
          {images.map((uri, i) => (
            <View key={`${uri}-${i}`} style={styles.thumbWrap}>
              <Image source={{ uri }} style={styles.thumb} />
              <TouchableOpacity
                style={styles.removeBtn}
                onPress={() => removeImage(i)}
                accessibilityRole="button"
                accessibilityLabel={`Remove image ${i + 1}`}
              >
                <Text style={styles.removeText}>×</Text>
              </TouchableOpacity>
            </View>
          ))}
        </ScrollView>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: 12 },
  buttons: { flexDirection: "row", gap: 8 },
  button: {
    flex: 1,
    backgroundColor: "#0E0E0C",
    borderRadius: 6,
    height: 44,
    alignItems: "center",
    justifyContent: "center",
  },
  buttonText: { color: "#F7F6F2", fontSize: 14, fontWeight: "600" },
  gallery: { marginTop: 8 },
  thumbWrap: { position: "relative", marginRight: 8 },
  thumb: { width: 80, height: 80, borderRadius: 6 },
  removeBtn: {
    position: "absolute",
    top: -6,
    right: -6,
    width: 22,
    height: 22,
    borderRadius: 11,
    backgroundColor: "#8B2020",
    alignItems: "center",
    justifyContent: "center",
  },
  removeText: { color: "#FFF", fontSize: 14, fontWeight: "700", marginTop: -1 },
});

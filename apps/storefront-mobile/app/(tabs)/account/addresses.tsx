import { useState, useCallback, useMemo } from "react";
import {
  View,
  Text,
  TextInput,
  StyleSheet,
  FlatList,
  Pressable,
  ActivityIndicator,
  Modal,
  ScrollView,
  Alert,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { MapPin, Plus, Pencil, Trash2 } from "lucide-react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import {
  useAddresses,
  useCreateAddress,
  useUpdateAddress,
  useDeleteAddress,
} from "@/lib/hooks/use-addresses";
import type { Address, AddressInput } from "@/lib/storefront-api/addresses";

const EMPTY_FORM: AddressInput = {
  first_name: "", last_name: "", line1: "", line2: "", city: "", state: "", postal_code: "", country: "", phone: "", is_default: false,
};

export default function AddressesScreen() {
  const theme = useTheme();
  const { data: addresses, isLoading } = useAddresses();
  const [modalVisible, setModalVisible] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<AddressInput>(EMPTY_FORM);

  const createMutation = useCreateAddress();
  const updateMutation = useUpdateAddress();
  const deleteMutation = useDeleteAddress();

  const themed = useMemo(() => ({
    container: { backgroundColor: theme.background },
    centered: { backgroundColor: theme.background },
    emptyTitle: { color: theme.text },
    emptySubtitle: { color: theme.textSecondary },
    addButton: { backgroundColor: theme.primary },
    addButtonText: { color: theme.elevated },
    card: { backgroundColor: theme.elevated, borderColor: theme.border },
    cardName: { color: theme.text },
    defaultBadgeText: { color: theme.accent },
    cardLine: { color: theme.textSecondary },
    cardPhone: { color: theme.textSecondary },
    cardActions: { borderTopColor: theme.border },
    actionLinkText: { color: theme.accent },
    fab: { backgroundColor: theme.primary },
    modalCard: { backgroundColor: theme.elevated },
    modalTitle: { color: theme.text },
    formLabel: { color: theme.text },
    formInput: { borderColor: theme.border, color: theme.text, backgroundColor: theme.elevated },
    saveButton: { backgroundColor: theme.primary },
    saveButtonText: { color: theme.elevated },
    cancelText: { color: theme.textSecondary },
  }), [theme]);

  const openCreate = useCallback(() => { setEditingId(null); setForm(EMPTY_FORM); setModalVisible(true); }, []);
  const openEdit = useCallback((address: Address) => {
    setEditingId(address.id);
    setForm({ first_name: address.first_name, last_name: address.last_name, line1: address.line1, line2: address.line2 ?? "", city: address.city, state: address.state, postal_code: address.postal_code, country: address.country, phone: address.phone ?? "", is_default: address.is_default });
    setModalVisible(true);
  }, []);

  const handleSave = async () => {
    if (!form.first_name.trim() || !form.line1.trim() || !form.city.trim() || !form.postal_code.trim() || !form.country.trim()) {
      Alert.alert("Missing fields", "Please fill in all required fields.");
      return;
    }
    if (editingId) { await updateMutation.mutateAsync({ id: editingId, body: form }); } else { await createMutation.mutateAsync(form); }
    setModalVisible(false);
  };

  const handleDelete = (id: string) => {
    Alert.alert("Delete address", "Are you sure you want to remove this address?", [
      { text: "Cancel", style: "cancel" },
      { text: "Delete", style: "destructive", onPress: () => deleteMutation.mutate(id) },
    ]);
  };

  const handleSetDefault = (id: string) => { updateMutation.mutate({ id, body: { is_default: true } }); };

  const updateField = useCallback(<K extends keyof AddressInput>(key: K, value: AddressInput[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  }, []);

  if (isLoading) {
    return (<View style={[styles.centered, themed.centered]}><ActivityIndicator size="large" color={theme.primary} /></View>);
  }

  if (!addresses || addresses.length === 0) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <MapPin size={48} color="#CCCCCC" />
        <Text style={[styles.emptyTitle, themed.emptyTitle]}>No saved addresses</Text>
        <Text style={[styles.emptySubtitle, themed.emptySubtitle]}>Add an address for faster checkout.</Text>
        <Pressable style={[styles.addButton, themed.addButton]} onPress={openCreate} accessibilityRole="button" accessibilityLabel="Add address">
          <Plus size={18} color={theme.elevated} />
          <Text style={[styles.addButtonText, themed.addButtonText]}>Add address</Text>
        </Pressable>
        <AddressFormModal visible={modalVisible} form={form} isEditing={editingId !== null} saving={createMutation.isPending || updateMutation.isPending} onUpdateField={updateField} onSave={handleSave} onDismiss={() => setModalVisible(false)} themed={themed} theme={theme} />
      </View>
    );
  }

  const renderItem = ({ item }: { item: Address }) => (
    <View style={[styles.card, themed.card]}>
      <View style={styles.cardHeader}>
        <Text style={[styles.cardName, themed.cardName]}>{item.first_name} {item.last_name}</Text>
        {item.is_default && (<View style={styles.defaultBadge}><Text style={[styles.defaultBadgeText, themed.defaultBadgeText]}>Default</Text></View>)}
      </View>
      <Text style={[styles.cardLine, themed.cardLine]}>{item.line1}</Text>
      {item.line2 ? <Text style={[styles.cardLine, themed.cardLine]}>{item.line2}</Text> : null}
      <Text style={[styles.cardLine, themed.cardLine]}>{item.city}, {item.state} {item.postal_code}</Text>
      <Text style={[styles.cardLine, themed.cardLine]}>{item.country}</Text>
      {item.phone ? <Text style={[styles.cardPhone, themed.cardPhone]}>{item.phone}</Text> : null}
      <View style={[styles.cardActions, themed.cardActions]}>
        {!item.is_default && (<Pressable onPress={() => handleSetDefault(item.id)} style={styles.actionLink} accessibilityRole="button" accessibilityLabel="Set as default"><Text style={[styles.actionLinkText, themed.actionLinkText]}>Set as default</Text></Pressable>)}
        <View style={styles.actionIcons}>
          <Pressable onPress={() => openEdit(item)} hitSlop={8} accessibilityRole="button" accessibilityLabel="Edit address"><Pencil size={18} color={theme.textSecondary} /></Pressable>
          <Pressable onPress={() => handleDelete(item.id)} hitSlop={8} accessibilityRole="button" accessibilityLabel="Delete address"><Trash2 size={18} color="#8B2020" /></Pressable>
        </View>
      </View>
    </View>
  );

  return (
    <View style={[styles.container, themed.container]}>
      <FlatList contentContainerStyle={styles.listContent} data={addresses} keyExtractor={(item) => item.id} renderItem={renderItem} />
      <Pressable style={[styles.fab, themed.fab]} onPress={openCreate} accessibilityRole="button" accessibilityLabel="Add address"><Plus size={24} color={theme.elevated} /></Pressable>
      <AddressFormModal visible={modalVisible} form={form} isEditing={editingId !== null} saving={createMutation.isPending || updateMutation.isPending} onUpdateField={updateField} onSave={handleSave} onDismiss={() => setModalVisible(false)} themed={themed} theme={theme} />
    </View>
  );
}

interface AddressFormModalProps {
  visible: boolean; form: AddressInput; isEditing: boolean; saving: boolean;
  onUpdateField: <K extends keyof AddressInput>(key: K, value: AddressInput[K]) => void;
  onSave: () => void; onDismiss: () => void;
  themed: Record<string, Record<string, string | undefined>>; theme: { textSecondary: string; elevated: string };
}

function AddressFormModal({ visible, form, isEditing, saving, onUpdateField, onSave, onDismiss, themed, theme }: AddressFormModalProps) {
  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onDismiss}>
      <KeyboardAvoidingView style={styles.modalOverlay} behavior={Platform.OS === "ios" ? "padding" : "height"}>
        <View style={[styles.modalCard, themed.modalCard]}>
          <ScrollView showsVerticalScrollIndicator={false}>
            <Text style={[styles.modalTitle, themed.modalTitle]}>{isEditing ? "Edit address" : "New address"}</Text>
            <View style={styles.modalRow}>
              <FormField label="First name" value={form.first_name} onChangeText={(v) => onUpdateField("first_name", v)} style={styles.halfField} themed={themed} placeholderColor={theme.textSecondary} />
              <FormField label="Last name" value={form.last_name} onChangeText={(v) => onUpdateField("last_name", v)} style={styles.halfField} themed={themed} placeholderColor={theme.textSecondary} />
            </View>
            <FormField label="Address line 1" value={form.line1} onChangeText={(v) => onUpdateField("line1", v)} themed={themed} placeholderColor={theme.textSecondary} />
            <FormField label="Address line 2" value={form.line2 ?? ""} onChangeText={(v) => onUpdateField("line2", v)} themed={themed} placeholderColor={theme.textSecondary} />
            <View style={styles.modalRow}>
              <FormField label="City" value={form.city} onChangeText={(v) => onUpdateField("city", v)} style={styles.halfField} themed={themed} placeholderColor={theme.textSecondary} />
              <FormField label="State" value={form.state} onChangeText={(v) => onUpdateField("state", v)} style={styles.halfField} themed={themed} placeholderColor={theme.textSecondary} />
            </View>
            <View style={styles.modalRow}>
              <FormField label="Postal code" value={form.postal_code} onChangeText={(v) => onUpdateField("postal_code", v)} style={styles.halfField} themed={themed} placeholderColor={theme.textSecondary} />
              <FormField label="Country" value={form.country} onChangeText={(v) => onUpdateField("country", v)} style={styles.halfField} themed={themed} placeholderColor={theme.textSecondary} />
            </View>
            <FormField label="Phone" value={form.phone ?? ""} onChangeText={(v) => onUpdateField("phone", v)} keyboardType="phone-pad" themed={themed} placeholderColor={theme.textSecondary} />
            <Pressable style={[styles.saveButton, themed.saveButton, saving && styles.saveButtonDisabled]} onPress={onSave} disabled={saving} accessibilityRole="button" accessibilityLabel={isEditing ? "Update address" : "Save address"}>
              {saving ? <ActivityIndicator size="small" color={theme.elevated} /> : <Text style={[styles.saveButtonText, themed.saveButtonText]}>{isEditing ? "Update" : "Save address"}</Text>}
            </Pressable>
            <Pressable style={styles.cancelButton} onPress={onDismiss} disabled={saving} accessibilityRole="button" accessibilityLabel="Cancel">
              <Text style={[styles.cancelText, themed.cancelText]}>Cancel</Text>
            </Pressable>
          </ScrollView>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

function FormField({ label, value, onChangeText, style, keyboardType, themed, placeholderColor }: {
  label: string; value: string; onChangeText: (text: string) => void; style?: object; keyboardType?: "default" | "phone-pad";
  themed: Record<string, Record<string, string | undefined>>; placeholderColor: string;
}) {
  return (
    <View style={[styles.formField, style]}>
      <Text style={[styles.formLabel, themed.formLabel]}>{label}</Text>
      <TextInput style={[styles.formInput, themed.formInput]} value={value} onChangeText={onChangeText} placeholderTextColor={placeholderColor} keyboardType={keyboardType ?? "default"} accessibilityLabel={label} />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  listContent: { padding: 16, gap: 12, paddingBottom: 80 },
  centered: { flex: 1, alignItems: "center", justifyContent: "center", paddingHorizontal: 32, gap: 10 },
  emptyTitle: { fontSize: 18, fontWeight: "700", marginTop: 12 },
  emptySubtitle: { fontSize: 14, textAlign: "center" },
  addButton: { flexDirection: "row", height: 44, borderRadius: 6, alignItems: "center", justifyContent: "center", paddingHorizontal: 20, gap: 8, marginTop: 8 },
  addButtonText: { fontSize: 15, fontWeight: "600" },
  card: { borderRadius: 6, padding: 16, gap: 2, borderWidth: StyleSheet.hairlineWidth },
  cardHeader: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginBottom: 6 },
  cardName: { fontSize: 15, fontWeight: "700" },
  defaultBadge: { backgroundColor: "#2D4A2B15", paddingHorizontal: 8, paddingVertical: 3, borderRadius: 4 },
  defaultBadgeText: { fontSize: 11, fontWeight: "600" },
  cardLine: { fontSize: 13, lineHeight: 18 },
  cardPhone: { fontSize: 13, marginTop: 4 },
  cardActions: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginTop: 12, paddingTop: 10, borderTopWidth: StyleSheet.hairlineWidth },
  actionLink: {},
  actionLinkText: { fontSize: 13, fontWeight: "500" },
  actionIcons: { flexDirection: "row", gap: 16 },
  fab: { position: "absolute", bottom: 24, right: 20, width: 52, height: 52, borderRadius: 26, alignItems: "center", justifyContent: "center", shadowColor: "#000", shadowOffset: { width: 0, height: 2 }, shadowOpacity: 0.15, shadowRadius: 6, elevation: 4 },
  modalOverlay: { flex: 1, backgroundColor: "rgba(0,0,0,0.4)", justifyContent: "flex-end" },
  modalCard: { borderTopLeftRadius: 16, borderTopRightRadius: 16, padding: 24, paddingBottom: 40, maxHeight: "85%" },
  modalTitle: { fontSize: 18, fontWeight: "700", marginBottom: 16 },
  modalRow: { flexDirection: "row", gap: 12 },
  halfField: { flex: 1 },
  formField: { marginBottom: 12, gap: 4 },
  formLabel: { fontSize: 13, fontWeight: "600" },
  formInput: { borderWidth: 1, borderRadius: 6, paddingHorizontal: 12, paddingVertical: 10, fontSize: 15 },
  saveButton: { height: 48, borderRadius: 6, alignItems: "center", justifyContent: "center", marginTop: 8 },
  saveButtonDisabled: { opacity: 0.6 },
  saveButtonText: { fontSize: 15, fontWeight: "600" },
  cancelButton: { alignItems: "center", paddingVertical: 12 },
  cancelText: { fontSize: 14 },
});

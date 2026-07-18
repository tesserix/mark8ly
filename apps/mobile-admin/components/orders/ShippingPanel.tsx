import { useCallback, useMemo, useRef, useState } from "react";
import { View, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import * as Haptics from "expo-haptics";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { ApiError } from "@repo/mobile-shared/api/client";
import { Text, Eyebrow, Hairline, StatusBadge, type StatusTone } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useShipment } from "@/lib/hooks/use-shipment";
import {
  useCreateShipment,
  useUpdateShipmentStatus,
  useRefreshTracking,
  useSchedulePickup,
  useDeleteShipment,
  useEmailLabel,
  useCancelShipment,
} from "@/lib/admin-api/shipment-actions";
import { EmailLabelSheet, type EmailLabelSheetHandle } from "./EmailLabelSheet";
import type { Shipment } from "@repo/mobile-shared/api/types";

// Shipping is only actionable while the order is live. Terminal/cancelled
// orders hide the panel entirely — mirrors the web ACTIONABLE_STATUSES.
const ACTIONABLE_ORDER_STATUSES = new Set(["pending", "confirmed", "fulfilled"]);

const SHIPMENT_TONE: Record<string, StatusTone> = {
  pending: "muted",
  created: "muted",
  in_transit: "info",
  out_for_delivery: "info",
  delivered: "success",
  exception: "danger",
};

// cancel_status vocabulary (shipments.go CancelStatus): requested is a
// carrier call in flight/queued, succeeded/failed/unsupported are terminal.
const CANCEL_TONE: Record<string, StatusTone> = {
  requested: "info",
  succeeded: "success",
  failed: "danger",
  unsupported: "warning",
};

// Advance-status forward order. A status can only move to a later step.
const STATUS_ORDER = ["pending", "created", "in_transit", "out_for_delivery", "delivered"];
const ADVANCE_STEPS: Array<{ status: string; label: string }> = [
  { status: "in_transit", label: "In transit" },
  { status: "out_for_delivery", label: "Out for delivery" },
  { status: "delivered", label: "Delivered" },
];

const KNOWN_CARRIERS = [
  { value: "delhivery", label: "Delhivery" },
  { value: "shipengine", label: "ShipEngine" },
  { value: "ninjavan", label: "NinjaVan" },
];
const KNOWN_SERVICES = [
  { value: "standard", label: "Standard" },
  { value: "express", label: "Express" },
];

function titleize(value: string): string {
  return value
    .split("_")
    .map((w) => (w ? w.charAt(0).toUpperCase() + w.slice(1) : w))
    .join(" ");
}

function formatDate(iso?: string): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleDateString("en-AU", { month: "short", day: "numeric", year: "numeric" });
}

function formatPickup(iso?: string): string | null {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  const day = d.toLocaleDateString("en-AU", { weekday: "short", month: "short", day: "numeric" });
  const time = d.toLocaleTimeString("en-AU", { hour: "2-digit", minute: "2-digit", hour12: false });
  return `${day}, ${time}`;
}

/** Merge a runtime value into a known option set so a carrier-specific service
 *  code (e.g. "auspost_parcel_post_australia") stays selectable/visible instead
 *  of being collapsed to a generic label. */
function withCurrent(
  options: Array<{ value: string; label: string }>,
  current: string,
): Array<{ value: string; label: string }> {
  if (!current || options.some((o) => o.value === current)) return options;
  return [{ value: current, label: titleize(current) }, ...options];
}

interface ShippingPanelProps {
  orderId: string;
  orderStatus: string;
  /** Carrier + service the customer picked at checkout (persisted on the
   *  order), used as the create-form defaults. Both optional (omitempty). */
  defaultCarrier?: string;
  defaultService?: string;
}

export function ShippingPanel({
  orderId,
  orderStatus,
  defaultCarrier,
  defaultService,
}: ShippingPanelProps) {
  const actionable = ACTIONABLE_ORDER_STATUSES.has(orderStatus);
  const { data: shipment, isLoading } = useShipment(orderId, actionable);

  if (!actionable) return null;

  return (
    <>
      <Eyebrow label="Shipping" style={styles.section} />
      <View style={styles.card}>
        {isLoading ? (
          <Text preset="body" color="textTertiary">
            Loading shipment…
          </Text>
        ) : shipment ? (
          <ShipmentDetails orderId={orderId} shipment={shipment} />
        ) : (
          <CreateShipment
            orderId={orderId}
            defaultCarrier={defaultCarrier}
            defaultService={defaultService}
          />
        )}
      </View>
    </>
  );
}

function DetailRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <View style={styles.detailRow}>
      <Text preset="caption" color="textTertiary">
        {label}
      </Text>
      <Text preset="body" color="text" style={mono ? styles.mono : undefined}>
        {value || "—"}
      </Text>
    </View>
  );
}

function ShipmentDetails({ orderId, shipment }: { orderId: string; shipment: Shipment }) {
  const emailSheetRef = useRef<EmailLabelSheetHandle>(null);
  const updateStatus = useUpdateShipmentStatus();
  const refresh = useRefreshTracking();
  const pickup = useSchedulePickup();
  const del = useDeleteShipment();
  const emailLabel = useEmailLabel();
  const cancelShipment = useCancelShipment();

  const isStub = shipment.tracking_number.startsWith("TEST-DLV-");
  const eta = formatDate(shipment.estimated_delivery);
  const pickupAt = formatPickup(shipment.pickup_scheduled_for);
  const carrierLabel = shipment.provider ? titleize(shipment.provider) : "the carrier";
  const alreadyCancelled = shipment.cancel_status === "succeeded";
  const busy =
    updateStatus.isPending ||
    refresh.isPending ||
    pickup.isPending ||
    del.isPending ||
    emailLabel.isPending ||
    cancelShipment.isPending;

  const curIndex = STATUS_ORDER.indexOf(shipment.status);
  const advance = useCallback(
    (status: string) => {
      updateStatus.mutate(
        { orderId, shipmentId: shipment.id, body: { status } },
        {
          onError: (err) =>
            Alert.alert(
              "Couldn't update shipment",
              err instanceof ApiError ? err.message : "Please try again.",
            ),
        },
      );
    },
    [orderId, shipment.id, updateStatus],
  );

  const onRefresh = useCallback(() => {
    refresh.mutate(
      { orderId, shipmentId: shipment.id },
      {
        onError: (err) =>
          Alert.alert(
            "Tracking refresh failed",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      },
    );
  }, [orderId, shipment.id, refresh]);

  const onSchedulePickup = useCallback(() => {
    Alert.alert("Schedule pickup", "Book a carrier pickup for the next business day?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Schedule",
        onPress: () =>
          pickup.mutate(
            { orderId, shipmentId: shipment.id },
            {
              onError: (err) =>
                Alert.alert(
                  "Reschedule failed",
                  err instanceof ApiError ? err.message : "Please try again.",
                ),
            },
          ),
      },
    ]);
  }, [orderId, shipment.id, pickup]);

  const onDelete = useCallback(() => {
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Warning);
    Alert.alert(
      "Delete shipment record",
      "Removes this shipment from the order so you can create a new label. It does NOT cancel or return anything with the carrier — use “Cancel / return shipment” for that.",
      [
        { text: "Cancel", style: "cancel" },
        {
          text: "Delete",
          style: "destructive",
          onPress: () =>
            del.mutate(
              { orderId, shipmentId: shipment.id },
              {
                onSuccess: () =>
                  void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success),
                onError: (err) => {
                  void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
                  Alert.alert(
                    "Couldn't delete shipment",
                    err instanceof ApiError ? err.message : "Please try again.",
                  );
                },
              },
            ),
        },
      ],
    );
  }, [orderId, shipment.id, del]);

  const onCancelReturn = useCallback(() => {
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Warning);
    Alert.alert(
      "Cancel / return shipment",
      `Ask ${carrierLabel} to cancel this shipment, or arrange a return if it's already been picked up? We'll show the outcome below.`,
      [
        { text: "Keep shipment", style: "cancel" },
        {
          text: "Cancel shipment",
          style: "destructive",
          onPress: () =>
            cancelShipment.mutate(
              { orderId, shipmentId: shipment.id },
              {
                onSuccess: (result) => {
                  void Haptics.notificationAsync(
                    result.status === "succeeded"
                      ? Haptics.NotificationFeedbackType.Success
                      : Haptics.NotificationFeedbackType.Warning,
                  );
                },
                onError: (err) => {
                  void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
                  Alert.alert(
                    "Couldn't cancel shipment",
                    err instanceof ApiError ? err.message : "Please try again.",
                  );
                },
              },
            ),
        },
      ],
    );
  }, [orderId, shipment.id, carrierLabel, cancelShipment]);

  const onEmailLabel = useCallback(
    (recipient: string) => {
      emailLabel.mutate(
        { orderId, shipmentId: shipment.id, recipient },
        {
          onSuccess: () => Alert.alert("Label sent", `Delivered to ${recipient}.`),
          onError: (err) =>
            Alert.alert(
              "Couldn't email label",
              err instanceof ApiError ? err.message : "Please try again.",
            ),
        },
      );
    },
    [orderId, shipment.id, emailLabel],
  );

  return (
    <View style={styles.detailsWrap}>
      <View style={styles.statusHeader}>
        <StatusBadge label={titleize(shipment.status)} tone={SHIPMENT_TONE[shipment.status] ?? "muted"} />
      </View>

      {shipment.provider ? <DetailRow label="Carrier" value={titleize(shipment.provider)} /> : null}
      {shipment.service ? <DetailRow label="Service" value={shipment.service} /> : null}
      <DetailRow label="Tracking" value={shipment.tracking_number || "Pending"} mono />
      {eta ? <DetailRow label="ETA" value={eta} /> : null}
      {pickupAt ? <DetailRow label="Pickup" value={pickupAt} /> : null}

      {isStub ? (
        <View style={styles.stubNote}>
          <Text preset="caption" color={theme.colors.warningInk}>
            This shipment uses a mock tracking number from an earlier test run and won&apos;t
            produce a real label. Delete it and create a new shipment.
          </Text>
        </View>
      ) : null}

      {shipment.cancel_status ? (
        <>
          <View style={styles.detailRow}>
            <Text preset="caption" color="textTertiary">
              Cancel / return
            </Text>
            <StatusBadge
              label={titleize(shipment.cancel_status)}
              tone={CANCEL_TONE[shipment.cancel_status] ?? "muted"}
            />
          </View>
          {shipment.cancel_reason ? (
            <Text preset="caption" color="textTertiary">
              {shipment.cancel_reason}
            </Text>
          ) : null}
          {shipment.cancel_status === "unsupported" ? (
            <View style={styles.stubNote}>
              <Text preset="caption" color={theme.colors.warningInk}>
                This carrier stage can&apos;t be auto-cancelled — arrange the return manually with{" "}
                {carrierLabel} using {shipment.tracking_number || "the tracking number"}.
              </Text>
            </View>
          ) : null}
        </>
      ) : null}

      <Hairline />

      <Text preset="caption" color="textTertiary">
        Advance status
      </Text>
      <View style={styles.chipRow}>
        {ADVANCE_STEPS.map((step) => {
          const nextIndex = STATUS_ORDER.indexOf(step.status);
          const disabled = busy || (curIndex >= 0 && nextIndex >= 0 && nextIndex <= curIndex);
          return (
            <ActionChip
              key={step.status}
              label={step.label}
              onPress={() => advance(step.status)}
              disabled={disabled}
            />
          );
        })}
      </View>

      <Hairline />

      <View style={styles.chipRow}>
        <ActionChip label="Email label" onPress={() => emailSheetRef.current?.present()} disabled={busy || isStub} />
        <ActionChip label={refresh.isPending ? "Syncing…" : "Refresh tracking"} onPress={onRefresh} disabled={busy || isStub} />
        <ActionChip label={pickup.isPending ? "Scheduling…" : "Schedule pickup"} onPress={onSchedulePickup} disabled={busy} />
      </View>

      {!alreadyCancelled ? (
        <>
          <Hairline />
          <Text preset="caption" color="textTertiary">
            Carrier cancel / return
          </Text>
          <View style={styles.chipRow}>
            <ActionChip
              label={cancelShipment.isPending ? "Cancelling…" : "Cancel / return shipment"}
              onPress={onCancelReturn}
              disabled={busy || isStub}
              tone="danger-solid"
            />
          </View>
        </>
      ) : null}

      <Hairline />
      <View style={styles.chipRow}>
        <ActionChip
          label={del.isPending ? "Deleting…" : "Delete shipment"}
          onPress={onDelete}
          disabled={busy}
          tone="muted"
        />
      </View>
      <Text preset="caption" color="textTertiary">
        Local record only — doesn&apos;t contact the carrier.
      </Text>

      <EmailLabelSheet ref={emailSheetRef} onSubmit={onEmailLabel} isSubmitting={emailLabel.isPending} />
    </View>
  );
}

function CreateShipment({
  orderId,
  defaultCarrier,
  defaultService,
}: {
  orderId: string;
  defaultCarrier?: string;
  defaultService?: string;
}) {
  const [open, setOpen] = useState(false);
  const initialProvider =
    defaultCarrier && KNOWN_CARRIERS.some((c) => c.value === defaultCarrier)
      ? defaultCarrier
      : defaultCarrier || "shipengine";
  const initialService = defaultService || "standard";
  const [provider, setProvider] = useState(initialProvider);
  const [service, setService] = useState(initialService);
  const create = useCreateShipment();

  const carrierOptions = useMemo(() => withCurrent(KNOWN_CARRIERS, provider), [provider]);
  const serviceOptions = useMemo(() => withCurrent(KNOWN_SERVICES, service), [service]);

  const onCreate = useCallback(() => {
    create.mutate(
      { orderId, body: { provider, service } },
      {
        onError: (err) =>
          Alert.alert(
            "Couldn't generate label",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
        onSuccess: () => setOpen(false),
      },
    );
  }, [orderId, provider, service, create]);

  if (!open) {
    return (
      <Pressable
        style={styles.createBtn}
        onPress={() => setOpen(true)}
        accessibilityRole="button"
        accessibilityLabel="Create shipping label"
      >
        <Text preset="bodyEmphasis" color="text">
          Create shipping label
        </Text>
      </Pressable>
    );
  }

  return (
    <View style={styles.createForm}>
      <Text preset="caption" color="textTertiary">
        Carrier
      </Text>
      <View style={styles.chipRow}>
        {carrierOptions.map((c) => (
          <SelectChip key={c.value} label={c.label} selected={c.value === provider} onPress={() => setProvider(c.value)} />
        ))}
      </View>

      <Text preset="caption" color="textTertiary">
        Service
      </Text>
      <View style={styles.chipRow}>
        {serviceOptions.map((s) => (
          <SelectChip key={s.value} label={s.label} selected={s.value === service} onPress={() => setService(s.value)} />
        ))}
      </View>

      <View style={styles.createActions}>
        <Pressable
          style={[styles.primaryBtn, (create.isPending || !provider || !service) && styles.disabled]}
          onPress={onCreate}
          disabled={create.isPending || !provider || !service}
          accessibilityRole="button"
          accessibilityLabel="Generate label"
        >
          {create.isPending ? (
            <ActivityIndicator size="small" color={theme.colors.inverse} />
          ) : (
            <Text preset="bodyEmphasis" color="inverse">
              Generate label
            </Text>
          )}
        </Pressable>
        <Pressable
          style={styles.ghostBtn}
          onPress={() => setOpen(false)}
          disabled={create.isPending}
          accessibilityRole="button"
          accessibilityLabel="Cancel"
        >
          <Text preset="bodyEmphasis" color="textSecondary">
            Cancel
          </Text>
        </Pressable>
      </View>
    </View>
  );
}

function ActionChip({
  label,
  onPress,
  disabled,
  tone = "default",
}: {
  label: string;
  onPress: () => void;
  disabled?: boolean;
  /** "danger-solid" is the primary carrier-cancel action (filled, high
   *  contrast). "muted" is the secondary local-record Delete — visually
   *  de-emphasized so the two don't read as duplicate destructive actions. */
  tone?: "default" | "danger" | "danger-solid" | "muted";
}) {
  const chipStyle = [
    styles.chip,
    tone === "danger-solid" && styles.chipDangerSolid,
    tone === "muted" && styles.chipMuted,
    disabled && styles.disabled,
  ];
  const textColor =
    tone === "danger-solid" ? "inverse" : tone === "danger" ? "danger" : tone === "muted" ? "textTertiary" : "text";
  return (
    <Pressable
      style={chipStyle}
      onPress={onPress}
      disabled={disabled}
      accessibilityRole="button"
      accessibilityLabel={label}
    >
      <Text preset="caption" color={textColor}>
        {label}
      </Text>
    </Pressable>
  );
}

function SelectChip({ label, selected, onPress }: { label: string; selected: boolean; onPress: () => void }) {
  return (
    <Pressable
      style={[styles.chip, selected && styles.chipSelected]}
      onPress={onPress}
      accessibilityRole="button"
      accessibilityState={{ selected }}
      accessibilityLabel={label}
    >
      <Text preset="caption" color={selected ? "inverse" : "text"}>
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  section: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl },
  card: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm },
  detailsWrap: { gap: theme.spacing.sm, paddingVertical: theme.spacing.sm },
  statusHeader: { flexDirection: "row" },
  detailRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", gap: theme.spacing.md },
  mono: { fontVariant: ["tabular-nums"] },
  stubNote: {
    backgroundColor: theme.colors.warningTint,
    borderRadius: theme.radii.sm,
    padding: theme.spacing.sm,
  },
  chipRow: { flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.xs },
  chip: {
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.md,
    minHeight: theme.touchTarget,
    justifyContent: "center",
  },
  chipSelected: { backgroundColor: theme.colors.text, borderColor: theme.colors.text },
  chipMuted: { borderColor: theme.colors.hairline },
  chipDangerSolid: { backgroundColor: theme.colors.danger, borderColor: theme.colors.danger },
  createBtn: {
    borderWidth: 1,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    height: 48,
    alignItems: "center",
    justifyContent: "center",
    marginVertical: theme.spacing.sm,
  },
  createForm: { gap: theme.spacing.sm, paddingVertical: theme.spacing.sm },
  createActions: { flexDirection: "row", gap: theme.spacing.md, marginTop: theme.spacing.sm },
  primaryBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  ghostBtn: {
    flex: 1,
    height: 48,
    borderRadius: theme.radii.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.4 },
});

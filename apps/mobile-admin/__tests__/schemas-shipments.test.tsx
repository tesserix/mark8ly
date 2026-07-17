import {
  shipmentSchema,
  shipmentOrNullSchema,
  SHIPMENT_ADVANCE_STATUSES,
} from "@repo/mobile-shared/api/schemas/shipments";

// A shipment that has progressed — every optional timestamp + pickup field
// present, matching a delivered, pickup-scheduled ShipmentResponse.
const FULL_SHIPMENT = {
  id: "sh1",
  order_id: "o1",
  provider: "delhivery",
  provider_shipment_id: "DLV-123",
  tracking_number: "AWB999",
  label_url: "https://cdn/label.pdf",
  service: "standard",
  status: "delivered",
  currency_code: "AUD",
  estimated_delivery: "2026-07-20T00:00:00Z",
  shipped_at: "2026-07-18T00:00:00Z",
  delivered_at: "2026-07-20T09:00:00Z",
  pickup_request_id: "PR-77",
  pickup_scheduled_for: "2026-07-17T14:00:00Z",
  created_at: "2026-07-17T00:00:00Z",
};

// A freshly-created shipment: the *time.Time + omitempty fields are ABSENT
// (Go omits them), and the un-omitempty strings arrive but can be empty.
const FRESH_SHIPMENT = {
  id: "sh2",
  order_id: "o2",
  provider: "shipengine",
  provider_shipment_id: "",
  tracking_number: "",
  label_url: "",
  service: "express",
  status: "pending",
  currency_code: "AUD",
  created_at: "2026-07-17T00:00:00Z",
};

describe("shipmentSchema", () => {
  it("parses a fully-progressed shipment with all optionals present", () => {
    const parsed = shipmentSchema.parse(FULL_SHIPMENT);
    expect(parsed.status).toBe("delivered");
    expect(parsed.pickup_scheduled_for).toBe("2026-07-17T14:00:00Z");
    expect(parsed.delivered_at).toBe("2026-07-20T09:00:00Z");
  });

  it("parses a fresh shipment where omitempty timestamps + pickup are ABSENT (not null)", () => {
    const parsed = shipmentSchema.parse(FRESH_SHIPMENT);
    expect(parsed.estimated_delivery).toBeUndefined();
    expect(parsed.shipped_at).toBeUndefined();
    expect(parsed.delivered_at).toBeUndefined();
    expect(parsed.pickup_request_id).toBeUndefined();
    expect(parsed.pickup_scheduled_for).toBeUndefined();
    // Un-omitempty strings are present-but-empty, and must be allowed through.
    expect(parsed.tracking_number).toBe("");
    expect(parsed.label_url).toBe("");
  });

  it("throws when a required field is missing (loud contract break)", () => {
    const { id: _omit, ...missingId } = FULL_SHIPMENT;
    expect(() => shipmentSchema.parse(missingId)).toThrow();
  });
});

describe("shipmentOrNullSchema (GET .../shipments)", () => {
  it("accepts JSON null — the no-shipment-yet response is 200-with-null, not 404", () => {
    expect(shipmentOrNullSchema.parse(null)).toBeNull();
  });

  it("still parses a real shipment", () => {
    const parsed = shipmentOrNullSchema.parse(FULL_SHIPMENT);
    expect(parsed?.id).toBe("sh1");
  });
});

describe("SHIPMENT_ADVANCE_STATUSES", () => {
  it("is the forward-progress trio the panel surfaces", () => {
    expect([...SHIPMENT_ADVANCE_STATUSES]).toEqual([
      "in_transit",
      "out_for_delivery",
      "delivered",
    ]);
  });
});

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/app/(admin)/settings/shipping/delivery-actions", () => ({
  setShippingMode: vi.fn(),
  saveDeliveryOptions: vi.fn(),
  createCourier: vi.fn(),
  updateCourier: vi.fn(),
  deleteCourier: vi.fn(),
}));
vi.mock("@/app/(admin)/settings/shipping/actions", () => ({
  removeShippingConfig: vi.fn(),
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock("@/components/feedback/Toaster", () => ({
  useToast: () => ({ toast: { success: vi.fn(), error: vi.fn() } }),
}));

import { ShippingSettingsClient } from "./ShippingSettingsClient";

describe("ShippingSettingsClient", () => {
  it("shows the mode switch even when no carriers are supported", () => {
    render(
      <ShippingSettingsClient
        supported={{ country_code: "ZZ", country_name: "Nowhere", shipping_carriers: [] } as never}
        configs={[]}
        warehouses={[]}
        editable
        storeCountry="ZZ"
        delivery={{ mode: "carrier", free_shipping_min: null, options: [], couriers: [] }}
      />,
    );
    expect(screen.getByLabelText("Own delivery")).toBeInTheDocument();
    expect(screen.getByText(/No shipping carriers are currently available/)).toBeInTheDocument();
  });
});

// use-add-option-handler.ts imports toOptionRequestBodies from OptionsEditor.tsx,
// which renders <OptionBuilderSheet> and therefore transitively imports
// lucide-react-native and @gorhom/bottom-sheet — mock both the same way
// options-editor.test.tsx does (see that file's comments for why).
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});
jest.mock("@gorhom/bottom-sheet", () => {
  const React = require("react");
  return {
    __esModule: true,
    BottomSheetModal: React.forwardRef((_props: unknown, ref: React.Ref<unknown>) => {
      React.useImperativeHandle(ref, () => ({ present: () => {}, dismiss: () => {} }));
      return null;
    }),
    BottomSheetView: ({ children }: { children?: React.ReactNode }) => children ?? null,
    BottomSheetScrollView: ({ children }: { children?: React.ReactNode }) => children ?? null,
  };
});
// `react-native`'s index.js does `require('./Libraries/Alert/Alert').default`
// (see node_modules/react-native/index.js), so the mock module needs a
// `default` key — a plain `{ alert: fn }` export leaves `Alert` undefined
// (same fix as __tests__/security.test.tsx).
jest.mock("react-native/Libraries/Alert/Alert", () => ({
  default: { alert: jest.fn() },
}));
jest.mock("expo-haptics", () => ({
  notificationAsync: jest.fn(),
  NotificationFeedbackType: { Success: "success" },
}));

import { renderHook } from "@testing-library/react-native";
import { Alert } from "react-native";
import * as Haptics from "expo-haptics";
import { useAddOptionHandler } from "@/lib/hooks/use-add-option-handler";

// Minimal ProductDetail-shaped fixture — same shape used by
// __tests__/option-matrix.test.tsx, which exhaustively covers buildOptionMatrix
// itself; this file only needs to confirm the handler wires it up correctly.
const product = {
  id: "p1",
  title: "Linen Shirt",
  options: [],
  variants: [
    {
      id: "v1",
      sku: "TBS-LS",
      price: 149,
      inventory_quantity: 5,
      currency_code: "AUD",
      option_values: [],
      position: 0,
    },
  ],
} as never;

describe("useAddOptionHandler", () => {
  beforeEach(() => {
    (Alert.alert as jest.Mock).mockClear();
    (Haptics.notificationAsync as jest.Mock).mockClear();
  });

  it("mutates with EXACTLY buildOptionMatrix's {options, variants} output — never a hand-built variants array", () => {
    const mutate = jest.fn();
    const { result } = renderHook(() =>
      useAddOptionHandler("p1", product, { mutate } as never),
    );

    result.current({ name: "Size", values: ["S", "M"] });

    expect(mutate).toHaveBeenCalledTimes(1);
    const [variables] = mutate.mock.calls[0] as [{ id: string; body: Record<string, unknown> }];
    expect(variables.id).toBe("p1");
    expect(variables.body.options).toEqual([{ name: "Size", values: ["S", "M"] }]);
    const variants = variables.body.variants as { id?: string; option_values: unknown[] }[];
    expect(variants).toHaveLength(2);
    // The existing variant's id must survive on the tuple it maps to — losing
    // it would silently soft-delete a real variant's price/stock/sales history.
    expect(variants.some((v) => v.id === "v1")).toBe(true);
    expect(Alert.alert).not.toHaveBeenCalled();
  });

  it("fires a success haptic when the option PATCH succeeds", () => {
    const mutate = jest.fn((_vars, opts) => {
      opts?.onSuccess?.();
    });
    const { result } = renderHook(() =>
      useAddOptionHandler("p1", product, { mutate } as never),
    );

    result.current({ name: "Size", values: ["S", "M"] });

    expect(Haptics.notificationAsync).toHaveBeenCalledWith("success");
    expect(Alert.alert).not.toHaveBeenCalled();
  });

  it("does NOT fire a success haptic when the PATCH fails", () => {
    const mutate = jest.fn((_vars, opts) => {
      opts?.onError?.(new Error("network down"));
    });
    const { result } = renderHook(() =>
      useAddOptionHandler("p1", product, { mutate } as never),
    );

    result.current({ name: "Size", values: ["S"] });

    expect(Haptics.notificationAsync).not.toHaveBeenCalled();
  });

  it("alerts instead of mutating when the axis is invalid (OptionMatrixError), never sending a partial matrix", () => {
    const mutate = jest.fn();
    const { result } = renderHook(() =>
      useAddOptionHandler("p1", product, { mutate } as never),
    );

    // buildOptionMatrix throws OptionMatrixError for a value-less option.
    result.current({ name: "Size", values: [] });

    expect(mutate).not.toHaveBeenCalled();
    expect(Alert.alert).toHaveBeenCalledWith("Can't add option", expect.any(String));
  });

  it("surfaces the mutation's own failure (network/API) as an alert, not silently", () => {
    const mutate = jest.fn((_vars, opts) => {
      opts?.onError?.(new Error("network down"));
    });
    const { result } = renderHook(() =>
      useAddOptionHandler("p1", product, { mutate } as never),
    );

    result.current({ name: "Size", values: ["S"] });

    expect(Alert.alert).toHaveBeenCalledWith(
      "Error",
      "Failed to add option. Please try again.",
    );
  });

  it("does nothing when the product hasn't loaded yet", () => {
    const mutate = jest.fn();
    const { result } = renderHook(() =>
      useAddOptionHandler("p1", undefined, { mutate } as never),
    );

    result.current({ name: "Size", values: ["S"] });

    expect(mutate).not.toHaveBeenCalled();
    expect(Alert.alert).not.toHaveBeenCalled();
  });
});

// The create screen renders CategoryField, which mounts a real
// @gorhom/bottom-sheet BottomSheetModal — that pulls in react-native-reanimated,
// which throws under jest without a full worklets/logger setup this project
// doesn't have. `@gorhom/bottom-sheet` is globally mapped in jest.config.js
// to lib/test-support/gorhom-bottom-sheet-mock.tsx (NOT under __tests__/, or
// jest-expo's default testMatch would try to run it as a test file with zero
// tests and fail) — unlike category-field.test.tsx, which never drives a
// real row selection, this file exercises the actual pick-a-category flow,
// which is exactly why that shared mock's BottomSheetFlatList maps `data`
// through `renderItem` like a real FlatList rather than stubbing it out.
// No local jest.mock() needed here (or anywhere else) any more — a local
// factory that itself `require()`s the same globally-mapped module recurses
// infinitely, since Jest resolves the mock factory's own module id through
// the same moduleNameMapper entry it's trying to mock.

// `@/components/ui`'s barrel re-exports SearchField/BackHeader, which import
// icons from lucide-react-native's ESM build — not covered by jest-expo's
// default transformIgnorePatterns, so requiring it unmocked throws
// "Unexpected token 'export'". Stub every icon export with a no-op component.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("expo-haptics", () => ({
  selectionAsync: jest.fn(),
  notificationAsync: jest.fn(),
  NotificationFeedbackType: { Success: "success" },
}));

// `@/components/ui`'s `Screen` calls `useSafeAreaInsets()`, which throws
// without a `<SafeAreaProvider>` ancestor. react-native-safe-area-context
// ships an official jest mock for exactly this (same fix as
// __tests__/TenantGate.test.tsx and __tests__/security.test.tsx).
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

const mockReplace = jest.fn();
jest.mock("expo-router", () => ({
  useRouter: () => ({ replace: mockReplace, back: jest.fn(), push: jest.fn() }),
}));

const mockCreateMutate = jest.fn();
const mockUpdateMutate = jest.fn();
const mockCategories = [
  { id: "cat-1", store_id: "s1", name: "Ceramics", slug: "ceramics", position: 0, is_active: true, featured: false, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];
jest.mock("@/lib/admin-api/product-crud", () => ({
  useCreateProduct: () => ({ mutate: mockCreateMutate, isPending: false }),
  useUpdateProduct: () => ({ mutate: mockUpdateMutate, isPending: false }),
  useCategories: () => ({
    data: mockCategories,
    isLoading: false,
    error: undefined,
    refetch: jest.fn(),
  }),
  useCreateCategory: () => ({ mutateAsync: jest.fn(), isPending: false }),
}));

import { render, fireEvent } from "@testing-library/react-native";
import { Alert } from "react-native";
import NewProductScreen from "../app/(tabs)/products/new";

// tsconfig scopes `types` to ["jest"] only, so Node's ambient globals aren't
// picked up automatically — declare the one the density-and-type describe
// block below needs (same fix as product-detail-sections.test.tsx).
declare const __dirname: string;

const FAKE_PRODUCT = { id: "product-1", title: "Ceramic Mug" };

beforeEach(() => {
  mockReplace.mockClear();
  mockCreateMutate.mockReset();
  mockUpdateMutate.mockReset();
  jest.spyOn(Alert, "alert").mockImplementation(() => {});
});

afterEach(() => {
  jest.restoreAllMocks();
});

describe("NewProductScreen — submission body", () => {
  it("submits title/price/status/variant with the derived SKU when SKU is left blank", () => {
    const { getByLabelText, getByText } = render(<NewProductScreen />);

    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");
    fireEvent.changeText(getByLabelText("Price (AUD)"), "24.50");
    fireEvent.changeText(getByLabelText("Stock"), "10");

    fireEvent.press(getByText("Create product"));

    expect(mockCreateMutate).toHaveBeenCalledTimes(1);
    const [body] = mockCreateMutate.mock.calls[0]!;
    expect(body).toEqual({
      title: "Ceramic Mug",
      description: undefined,
      status: "draft",
      variants: [
        {
          sku: "CERAMIC-MUG-1",
          price: 24.5,
          currency_code: "AUD",
          inventory_quantity: 10,
          position: 0,
        },
      ],
    });
  });

  it("uses the entered SKU when one is provided, and honours the Active status toggle", () => {
    const { getByLabelText, getByText } = render(<NewProductScreen />);

    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");
    fireEvent.changeText(getByLabelText("Price (AUD)"), "24.50");
    fireEvent.changeText(getByLabelText("SKU"), "MUG-001");
    fireEvent.press(getByText("Active"));

    fireEvent.press(getByText("Create product"));

    const [body] = mockCreateMutate.mock.calls[0]!;
    expect(body.status).toBe("active");
    expect(body.variants[0].sku).toBe("MUG-001");
  });

  it("defaults stock to 0 when left blank", () => {
    const { getByLabelText, getByText } = render(<NewProductScreen />);
    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");
    fireEvent.changeText(getByLabelText("Price (AUD)"), "24.50");

    fireEvent.press(getByText("Create product"));

    const [body] = mockCreateMutate.mock.calls[0]!;
    expect(body.variants[0].inventory_quantity).toBe(0);
  });

  it("'Save as draft' forces status to draft regardless of the segmented control", () => {
    const { getByLabelText, getByText } = render(<NewProductScreen />);
    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");
    fireEvent.changeText(getByLabelText("Price (AUD)"), "24.50");
    fireEvent.press(getByText("Active"));

    fireEvent.press(getByText("Save as draft"));

    const [body] = mockCreateMutate.mock.calls[0]!;
    expect(body.status).toBe("draft");
  });
});

describe("NewProductScreen — inline validation", () => {
  it("blocks submission and shows an inline error when title is empty — no Alert", () => {
    const { getByLabelText, getByText, queryByText } = render(<NewProductScreen />);
    fireEvent.changeText(getByLabelText("Price (AUD)"), "10");

    fireEvent.press(getByText("Create product"));

    expect(mockCreateMutate).not.toHaveBeenCalled();
    expect(Alert.alert).not.toHaveBeenCalled();
    expect(queryByText("Title is required.")).toBeTruthy();
  });

  it("blocks submission and shows an inline error when price is negative — no Alert", () => {
    const { getByLabelText, getByText, queryByText } = render(<NewProductScreen />);
    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");
    fireEvent.changeText(getByLabelText("Price (AUD)"), "-5");

    fireEvent.press(getByText("Create product"));

    expect(mockCreateMutate).not.toHaveBeenCalled();
    expect(Alert.alert).not.toHaveBeenCalled();
    expect(queryByText("Enter a valid, non-negative price.")).toBeTruthy();
  });

  it("blocks submission when price is blank", () => {
    const { getByLabelText, getByText } = render(<NewProductScreen />);
    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");

    fireEvent.press(getByText("Create product"));

    expect(mockCreateMutate).not.toHaveBeenCalled();
  });
});

describe("NewProductScreen — hand-off on success", () => {
  it("replaces to the edit screen with the new id and created=1 when no category was picked", () => {
    mockCreateMutate.mockImplementation((_body, opts) => {
      opts.onSuccess(FAKE_PRODUCT);
    });
    const { getByLabelText, getByText } = render(<NewProductScreen />);
    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");
    fireEvent.changeText(getByLabelText("Price (AUD)"), "24.50");

    fireEvent.press(getByText("Create product"));

    expect(mockUpdateMutate).not.toHaveBeenCalled();
    expect(mockReplace).toHaveBeenCalledWith({
      pathname: "/(tabs)/products/[id]",
      params: { id: "product-1", created: "1" },
    });
  });

  it("surfaces a create failure via Alert and does not navigate", () => {
    mockCreateMutate.mockImplementation((_body, opts) => {
      opts.onError(new Error("network down"));
    });
    const { getByLabelText, getByText } = render(<NewProductScreen />);
    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");
    fireEvent.changeText(getByLabelText("Price (AUD)"), "24.50");

    fireEvent.press(getByText("Create product"));

    expect(Alert.alert).toHaveBeenCalled();
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("fires the category PATCH before replacing when a category was selected, then replaces on its success", () => {
    mockCreateMutate.mockImplementation((_body, opts) => {
      opts.onSuccess(FAKE_PRODUCT);
    });
    mockUpdateMutate.mockImplementation((_args, opts) => {
      opts.onSuccess();
    });

    const { getByLabelText, getByText } = render(<NewProductScreen />);
    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");
    fireEvent.changeText(getByLabelText("Price (AUD)"), "24.50");

    // Select "Ceramics" in the (real, unmocked-at-this-level) CategoryField ->
    // CategoryPickerSheet tree, then commit with Done — the sheet mock above
    // renders the sheet's content unconditionally, so these rows are already
    // in the tree without needing to "open" anything.
    fireEvent.press(getByLabelText("Ceramics"));
    fireEvent.press(getByLabelText("Done"));

    fireEvent.press(getByText("Create product"));

    expect(mockCreateMutate).toHaveBeenCalledTimes(1);
    expect(mockUpdateMutate).toHaveBeenCalledTimes(1);
    const [args] = mockUpdateMutate.mock.calls[0]!;
    expect(args).toEqual({ id: "product-1", body: { category_ids: ["cat-1"] } });
    expect(mockReplace).toHaveBeenCalledWith({
      pathname: "/(tabs)/products/[id]",
      params: { id: "product-1", created: "1" },
    });
  });

  it("still replaces to edit, with an alert, when the follow-up category PATCH fails", () => {
    mockCreateMutate.mockImplementation((_body, opts) => {
      opts.onSuccess(FAKE_PRODUCT);
    });
    mockUpdateMutate.mockImplementation((_args, opts) => {
      opts.onError(new Error("categories failed"));
    });

    const { getByLabelText, getByText } = render(<NewProductScreen />);
    fireEvent.changeText(getByLabelText("Title"), "Ceramic Mug");
    fireEvent.changeText(getByLabelText("Price (AUD)"), "24.50");
    fireEvent.press(getByLabelText("Ceramics"));
    fireEvent.press(getByLabelText("Done"));

    fireEvent.press(getByText("Create product"));

    expect(mockUpdateMutate).toHaveBeenCalledTimes(1);
    expect(Alert.alert).toHaveBeenCalled();
    expect(mockReplace).toHaveBeenCalledWith({
      pathname: "/(tabs)/products/[id]",
      params: { id: "product-1", created: "1" },
    });
  });
});

describe("NewProductScreen — density and type (task 10)", () => {
  const source = require("fs").readFileSync(
    require("path").join(__dirname, "../app/(tabs)/products/new.tsx"),
    "utf8",
  );

  it("widens the card gutter to the screen's own 20pt inset", () => {
    expect(source).toMatch(/card:\s*\{\s*marginHorizontal:\s*theme\.spacing\.xl,/);
  });

  it("defines a screen-level eyebrowGutter style at the card's own 20pt gutter", () => {
    expect(source).toContain("eyebrowGutter: { paddingHorizontal: theme.spacing.xl },");
  });

  it("gives all three eyebrows (Essentials, Status, Category) the same 20pt gutter", () => {
    expect(source.match(/style=\{styles\.eyebrowGutter\}/g)?.length).toBe(3);
  });
});

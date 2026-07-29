// Defect 2's guard: the customer DETAIL screen printed the same email three
// times — in the back-header title, as the identity h2, and again as the
// subtitle beneath it — and broke the h2 mid-address.
//
// Rendered, not source-grepped. The sibling `customer-detail-sections.test.tsx`
// asserts against the file's TEXT, which cannot tell "renders the email once"
// from "renders it three times"; the whole point of this guard is the count.
//
// The no-name customer is the case. A customer WITH a name passes against the
// broken code, so the named case below is only here to prove the email did not
// disappear from the screen that should still show it.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

// No local `jest.mock("@gorhom/bottom-sheet", …)`. It used to stub three
// exports as bare `View`s, which is a divergent fourth copy of a mock the
// jest config already maps globally (`lib/test-support/gorhom-bottom-sheet-mock`)
// — and it silently omitted whichever exports the sheet had not needed YET.
// `BlockReasonSheet` gaining a `BottomSheetScrollView` body (so its actions
// stay reachable at accessibility text sizes) made every test in this file
// throw `Cannot read properties of undefined (reading 'displayName')`, because
// the missing export arrived as `undefined` and NativeWind's JSX interop reads
// `displayName` off every component type. The mapped mock carries the whole
// surface, so it cannot go stale that way.

const mockBack = jest.fn();
jest.mock("expo-router", () => ({
  useRouter: () => ({ back: mockBack, push: jest.fn() }),
  useLocalSearchParams: () => ({ id: "c-real" }),
}));

jest.mock("@repo/mobile-shared/stores/tenant-store", () => ({
  useTenantStore: (selector: (s: unknown) => unknown) =>
    selector({ activeStore: { id: "s1", name: "Bondi Supply", currency_code: "AUD" } }),
}));

let mockCustomer: unknown = null;
jest.mock("@/lib/hooks/use-customers", () => ({
  useCustomer: () => ({ data: mockCustomer, isLoading: false, error: null }),
}));

jest.mock("@/lib/admin-api/customer-actions", () => ({
  useBlockCustomer: () => ({ mutate: jest.fn(), isPending: false }),
  useUnblockCustomer: () => ({ mutate: jest.fn(), isPending: false }),
}));

import { render } from "@testing-library/react-native";
import CustomerDetailScreen from "../app/(tabs)/customers/[id]";
import { theme } from "@/lib/theme";

const NO_NAME_EMAIL = "mahesh.sangawar@gmail.com";

const BASE = {
  id: "c-real",
  email: NO_NAME_EMAIL,
  tags: [],
  status: "active",
  marketing_opt_in: false,
  order_count: 0,
  total_spent: 0,
  addresses: [],
  created_at: "2026-05-06T00:00:00Z",
  updated_at: "2026-05-06T00:00:00Z",
};

afterEach(() => {
  mockCustomer = null;
});

describe("customer detail — the identity is stated ONCE", () => {
  it("renders a no-name customer's email exactly once on the whole screen", () => {
    mockCustomer = BASE;
    const { getAllByText } = render(<CustomerDetailScreen />);
    // Was three: back-header title, h2, subtitle. Restoring ANY of the three
    // (a `title={identity.title}` on BackHeader, or an unconditional
    // `<Text>{customer.email}</Text>` subtitle) makes this 2 or 3.
    expect(getAllByText(NO_NAME_EMAIL)).toHaveLength(1);
  });

  it("puts that single copy in the h2 identity title, not in the back bar", () => {
    mockCustomer = BASE;
    const { getByTestId, queryByTestId } = render(<CustomerDetailScreen />);
    expect(getByTestId("customer-detail-title").props.children).toBe(NO_NAME_EMAIL);
    expect(queryByTestId("customer-detail-subtitle")).toBeNull();
  });

  it("still shows the name AND the email for a customer who has a name", () => {
    mockCustomer = { ...BASE, first_name: "Ada", last_name: "Lovelace" };
    const { getByTestId } = render(<CustomerDetailScreen />);
    expect(getByTestId("customer-detail-title").props.children).toBe("Ada Lovelace");
    expect(getByTestId("customer-detail-subtitle").props.children).toBe(NO_NAME_EMAIL);
  });
});

describe("customer detail — an email title never breaks mid-address", () => {
  // An email has no wrap boundary: split anywhere it reads as if the address
  // ended at the line break (`mahesh.sangawar@gmai` / `l.com`). So it gets ONE
  // line and shrinks to a caption-sized floor — the CollapsingHeader
  // mechanism, with the line allowance deliberately withheld. A real name is
  // prose and still gets two lines to wrap in.
  it("gives an email title one line and a real shrink floor", () => {
    mockCustomer = BASE;
    const { getByTestId } = render(<CustomerDetailScreen />);
    const title = getByTestId("customer-detail-title");
    expect(title.props.numberOfLines).toBe(1);
    expect(title.props.adjustsFontSizeToFit).toBe(true);
    // 13pt (caption) as a fraction of h2 — NOT a bare 0.5, which would
    // authorise a 12pt identity line at the default text size.
    expect(title.props.minimumFontScale).toBeCloseTo(
      theme.text.caption.fontSize / theme.text.h2.fontSize,
    );
    expect(title.props.minimumFontScale).not.toBe(0.5);
    expect(title.props.minimumFontScale * theme.text.h2.fontSize).toBeGreaterThanOrEqual(11);
  });

  it("gives a real NAME two lines instead, because prose can wrap at a space", () => {
    mockCustomer = { ...BASE, first_name: "Ada", last_name: "Lovelace" };
    const { getByTestId } = render(<CustomerDetailScreen />);
    expect(getByTestId("customer-detail-title").props.numberOfLines).toBe(2);
  });
});

describe("customer detail — the monogram matches every other screen's", () => {
  it("is a rounded square at Thumb's radius, not a circle", () => {
    mockCustomer = BASE;
    const { getByTestId } = render(<CustomerDetailScreen />);
    const { StyleSheet } = require("react-native");
    const style = StyleSheet.flatten(getByTestId("customer-detail-monogram").props.style);
    expect(style.borderRadius).toBe(theme.radii.md);
    expect(style.borderRadius).not.toBe(style.width / 2);
  });
});

// TenantMonogram sits in `CollapsingHeader`'s `rightSlot`, and both of the
// glyphs it draws live in boxes whose size is FIXED in points and does not
// scale with Dynamic Type: a 40pt disc and a 16pt unread badge. Neither can
// be capped by the app-wide 200% default in `Text.tsx` — 200% of a 26pt h3
// does not fit a 40pt disc, and 200% of a 12pt line does not fit a 16pt
// chip. Both therefore pin their own tighter cap, and both of those pins are
// invisible to every other test in the suite.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("expo-router", () => ({ useRouter: () => ({ push: jest.fn() }) }));

let mockUnreadCount = 0;
jest.mock("@/lib/hooks/use-notifications", () => ({
  useNotifications: () => ({
    data: {
      notifications: Array.from({ length: mockUnreadCount }, (_, i) => ({
        id: `n${i}`,
        is_read: false,
      })),
    },
  }),
}));

import { render } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { TenantMonogram } from "@/components/dashboard/TenantMonogram";

beforeEach(() => {
  mockUnreadCount = 0;
});

describe("TenantMonogram — fixed-size boxes pin their own Dynamic Type cap", () => {
  it("pins the store initial at 1×, because a 40pt disc has no content to resize", () => {
    const { getByText } = render(<TenantMonogram storeName="The Bondi Store" />);
    expect(getByText("T").props.maxFontSizeMultiplier).toBe(1);
  });

  // The badge is `height: BADGE_MIN` (16) holding a 12pt line box, so it
  // clipped from ~1.33× — long before the app-wide cap could have helped.
  it("pins the unread badge at 1×, because a 16pt chip clips from 1.33×", () => {
    mockUnreadCount = 3;
    const { getByTestId, getByText } = render(<TenantMonogram storeName="Bondi" />);
    expect(getByTestId("tenant-monogram-badge")).toBeTruthy();
    expect(getByText("3").props.maxFontSizeMultiplier).toBe(1);
  });

  // The count is not LOST by capping the badge: the pressable's own label
  // speaks it, which is the surface a screen reader and an accessibility
  // audit actually read.
  it("keeps the unread count reachable through the button's accessibility label", () => {
    mockUnreadCount = 3;
    const { getByTestId } = render(<TenantMonogram storeName="Bondi" />);
    expect(getByTestId("tenant-monogram").props.accessibilityLabel).toBe(
      "Notifications, 3 unread",
    );
  });

  it("caps the badge to 9+ so the chip holds at most two glyphs", () => {
    mockUnreadCount = 42;
    const { getByText } = render(<TenantMonogram storeName="Bondi" />);
    expect(getByText("9+")).toBeTruthy();
  });

  it("renders no badge at all when nothing is unread", () => {
    const { queryByTestId } = render(<TenantMonogram storeName="Bondi" />);
    expect(queryByTestId("tenant-monogram-badge")).toBeNull();
  });

  // The disc's own line height is pinned to the disc so the glyph stays
  // optically centred; if that ever drifts the initial rides high or low.
  it("pins the initial's line box to the disc diameter", () => {
    const { getByText } = render(<TenantMonogram storeName="Bondi" />);
    const style = StyleSheet.flatten(getByText("B").props.style) as { lineHeight?: number };
    expect(style.lineHeight).toBe(40);
  });
});

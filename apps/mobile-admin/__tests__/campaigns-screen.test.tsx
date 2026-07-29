// Campaigns, brought onto the increment-3 native-UX kit (inc3 Task 7):
// collapsing header with a back chevron, and a two-item long-press menu whose
// only real action is an irreversible Delete.
//
// NO SWIPE. `DELETE /campaigns/:id` is legal for a DRAFT and nothing else
// (campaign/service.go:197-205 → 409 `campaign_not_draft`), and a second call
// 404s. An action that is illegal on most rows and unrecoverable on the rest
// is the worst possible fit for a one-thumb gesture, so the only route to it
// is a long press followed by a confirm.
//
// The FIXTURES carry the real risk here. A draft-only store passes against a
// MISSING status gate — every item would look correctly enabled — so a
// non-draft campaign is mandatory, and there are two of them (`sent` and
// `scheduled`) because the gate is `status !== "draft"` rather than a single
// forbidden value.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

// Local mock (it wins over `__mocks__/react-native-reanimated.js`): this
// screen drives its `CollapsingHeader` from an `Animated.FlatList`, which the
// global mock does not carry. Same shape as customers-screen.test.tsx's.
jest.mock("react-native-reanimated", () => {
  const { View, FlatList } = require("react-native");
  const { useRef } = require("react");
  function interpolate(
    value: number,
    inputRange: [number, number],
    outputRange: [number, number],
  ) {
    const [inMin, inMax] = inputRange;
    const [outMin, outMax] = outputRange;
    const t = Math.max(0, Math.min(1, (value - inMin) / (inMax - inMin)));
    return outMin + t * (outMax - outMin);
  }
  return {
    __esModule: true,
    default: { View, FlatList },
    Easing: { bezier: () => (t: number) => t },
    Extrapolation: { CLAMP: "clamp", EXTEND: "extend", IDENTITY: "identity" },
    interpolate,
    useAnimatedStyle: (factory: () => unknown) => factory(),
    useDerivedValue: (factory: () => number) => ({ value: factory() }),
    useAnimatedScrollHandler: (handler: unknown) => handler,
    useSharedValue: (initial: number) => {
      const ref = useRef(undefined) as { current: { value: number } | undefined };
      if (ref.current === undefined) ref.current = { value: initial };
      return ref.current;
    },
    useReducedMotion: jest.fn(() => false),
  };
});

const mockPush = jest.fn();
const mockBack = jest.fn();
jest.mock("expo-router", () => ({
  useRouter: () => ({ push: mockPush, back: mockBack }),
}));

jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    actionSucceeded: jest.fn(() => Promise.resolve()),
    actionFailed: jest.fn(() => Promise.resolve()),
    swipeThreshold: jest.fn(() => Promise.resolve()),
    menuOpen: jest.fn(() => Promise.resolve()),
    selectionChanged: jest.fn(() => Promise.resolve()),
  },
}));

interface CampaignsQueryState {
  campaigns: Campaign[];
  isLoading: boolean;
  isError: boolean;
}
const mockState: CampaignsQueryState = { campaigns: [], isLoading: false, isError: false };
const mockRefetch = jest.fn();
const mockListParams: (unknown | undefined)[] = [];

jest.mock("@/lib/hooks/use-campaigns", () => ({
  useCampaigns: (params?: { status?: string }) => {
    mockListParams.push(params);
    const rows = params?.status
      ? mockState.campaigns.filter((c) => c.status === params.status)
      : mockState.campaigns;
    return {
      data: { pages: [{ data: rows, meta: { page: 1, total: rows.length, total_pages: 1 } }] },
      isLoading: mockState.isLoading,
      isRefetching: false,
      isError: mockState.isError,
      refetch: mockRefetch,
      fetchNextPage: jest.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    };
  },
}));

const mockDelete = jest.fn();
jest.mock("@/lib/admin-api/campaign-actions", () => ({
  useDeleteCampaign: () => ({ mutate: mockDelete, isPending: false }),
}));

import { act, fireEvent, render } from "@testing-library/react-native";
import { Alert, StyleSheet } from "react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import CampaignsScreen from "../app/(tabs)/more/marketing/campaigns/index";
import { CampaignRow } from "@/components/marketing/CampaignRow";
import { theme } from "@/lib/theme";
import { swipeRows, type Root } from "../test-utils/swipe-convention";
import type { Campaign } from "@repo/mobile-shared/api/types";

function campaign(over: Partial<Campaign> = {}): Campaign {
  return {
    id: "cam-draft",
    name: "Winter warmers",
    type: "email",
    status: "draft",
    subject: "Cosy picks for July",
    content: null,
    segment_id: null,
    coupon_id: null,
    scheduled_at: null,
    sent_at: null,
    total_recipients: 0,
    delivered: 0,
    opened: 0,
    clicked: 0,
    converted: 0,
    unsubscribed: 0,
    failed: 0,
    revenue: 0,
    show_on_storefront: false,
    storefront_priority: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  } as unknown as Campaign;
}

/** The ONE shape on which Delete is legal. */
const DRAFT = campaign();
/**
 * A campaign that has already gone out. Deleting it is a guaranteed 409, and
 * a draft-only fixture set would pass against a screen with no gate at all.
 */
const SENT = campaign({
  id: "cam-sent",
  name: "Spring launch",
  status: "sent",
  delivered: 1204,
  opened: 388,
  sent_at: "2026-03-02T00:00:00Z",
});
/**
 * The second illegal shape. The gate is `status !== "draft"`, not "not sent",
 * so one non-draft fixture cannot tell a correct gate from `status === "sent"`.
 */
const SCHEDULED = campaign({
  id: "cam-scheduled",
  name: "Easter teaser",
  status: "scheduled",
  scheduled_at: "2026-04-01T00:00:00Z",
});

beforeEach(() => {
  jest.clearAllMocks();
  mockListParams.length = 0;
  mockState.campaigns = [DRAFT, SENT, SCHEDULED];
  mockState.isLoading = false;
  mockState.isError = false;
});

/** Opens the long-press menu on a given row. */
function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`campaign-row-${id}`) as never, "longPress");
}

/**
 * One row's mounted `CampaignRow` element.
 *
 * The busy guard is read off `onLongPress` THROUGH THIS, not through "firing
 * longPress produced no sheet": `fireEvent(el, "longPress")` against an
 * element with no handler is a silent no-op, so that proxy passes both when
 * the guard holds and when the event never dispatched at all.
 */
function campaignRow(root: Root, id: string) {
  return root
    .findAllByType(CampaignRow)
    .find((n) => (n.props as { campaign: Campaign }).campaign.id === id);
}

/**
 * Drives the destructive button of the Nth `Alert.alert` confirm.
 *
 * Found by `style: "destructive"`, never by index: a confirm whose buttons
 * were reordered so Cancel came second would still pass an index lookup.
 */
function acceptConfirm(spy: jest.SpyInstance, call = 0) {
  const buttons = spy.mock.calls[call][2] as { style?: string; onPress?: () => void }[];
  const destructive = buttons.find((b) => b.style === "destructive");
  expect(destructive).toBeDefined();
  act(() => destructive?.onPress?.());
}

describe("Campaigns — the collapsing header", () => {
  it("replaces BackHeader with a CollapsingHeader carrying the eyebrow and title", () => {
    const { getByTestId, getAllByText } = render(<CampaignsScreen />);
    expect(getByTestId("collapsing-header")).toBeTruthy();
    // Both layers are always mounted and cross-faded, so the title appears
    // twice — the proof it is THIS primitive and not the old flat header.
    expect(getAllByText("Campaigns")).toHaveLength(2);
    expect(getAllByText("MARKETING")).toHaveLength(1);
  });

  it("keeps a back affordance, and it navigates back", () => {
    const { getByLabelText } = render(<CampaignsScreen />);
    fireEvent.press(getByLabelText("Go back"));
    expect(mockBack).toHaveBeenCalledTimes(1);
  });

  // A NESTED route, so the chevron gets its own nav row above the editorial
  // block and the title starts at the screen gutter with the chips and rows.
  it("gives the chevron its own nav row rather than indenting the title", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    expect(getByTestId("collapsing-header-nav-row")).toBeTruthy();
  });

  it("keeps the New campaign action in the trailing slot", () => {
    const { getByLabelText } = render(<CampaignsScreen />);
    fireEvent.press(getByLabelText("New campaign"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/campaigns/new");
  });

  // The header only collapses if something feeds its `scrollY`, and only a
  // reanimated scroll view can. A plain `FlatList` would leave it frozen
  // expanded forever — a bug no row assertion can see.
  it("drives the header from an animated list, throttled at 16ms", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    const list = getByTestId("campaigns-list");
    expect(list.props.onScroll).toEqual(expect.any(Function));
    expect(list.props.scrollEventThrottle).toBe(16);
  });

  it("keeps the filter chips OUTSIDE the list, not in its header", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    expect(getByTestId("campaigns-list").props.ListHeaderComponent).toBeFalsy();
    expect(getByTestId("filter-chip-draft")).toBeTruthy();
  });
});

describe("Campaigns — the long-press menu", () => {
  const KEYS = ["edit", "delete"];

  // `snapPoints` memoises on `items.length`, so a dropped item resizes the
  // sheet under the merchant's thumb. Illegal actions are DISABLED, never
  // dropped — asserted on every status shape, not just the legal one.
  it("renders exactly two items on every campaign shape", () => {
    const { getByTestId, getAllByTestId, queryAllByTestId } = render(<CampaignsScreen />);
    for (const id of ["cam-draft", "cam-sent", "cam-scheduled"]) {
      longPress(getByTestId, id);
      expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(2);
      for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();
      // Close it before the next shape — `ActionSheet` only re-presents on a
      // false→true edge.
      fireEvent.press(getByTestId("action-sheet-item-edit"));
      expect(queryAllByTestId(/^action-sheet-item-/)).toHaveLength(0);
    }
  });

  it("titles the sheet with the campaign's own name", () => {
    const { getByTestId, getAllByText } = render(<CampaignsScreen />);
    const rowOnly = getAllByText("Spring launch").length;
    longPress(getByTestId, "cam-sent");
    expect(getAllByText("Spring launch")).toHaveLength(rowOnly + 1);
  });

  it("opens the campaign detail from Edit", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-draft");
    fireEvent.press(getByTestId("action-sheet-item-edit"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/campaigns/cam-draft");
  });

  // There is no duplicate endpoint on the admin API. An item that can only
  // ever 404 is worse than no item at all.
  it("offers no Duplicate item (no endpoint exists)", () => {
    const { getByTestId, getAllByTestId, queryByText } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-draft");
    const keys = getAllByTestId(/^action-sheet-item-/).map(
      (el) => (el.props as { testID: string }).testID,
    );
    expect(keys).toEqual(["action-sheet-item-edit", "action-sheet-item-delete"]);
    expect(queryByText(/duplicate/i)).toBeNull();
  });

  it("paints Delete as the destructive item, not merely labels it", () => {
    const { getByText, getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-draft");
    expect(StyleSheet.flatten(getByText("Delete").props.style).color).toBe(theme.colors.danger);
    // Discriminating: its sibling in the same sheet is NOT painted danger.
    expect(StyleSheet.flatten(getByText("Edit").props.style)?.color).not.toBe(
      theme.colors.danger,
    );
  });
});

describe("Campaigns — Delete is gated on draft", () => {
  it("enables Delete on a draft campaign", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-draft");
    expect(getByTestId("action-sheet-item-delete").props.accessibilityState.disabled).toBe(false);
  });

  it("disables Delete on a SENT campaign rather than dropping it", () => {
    const { getByTestId, getAllByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-sent");
    expect(getByTestId("action-sheet-item-delete").props.accessibilityState.disabled).toBe(true);
    // Present-but-unavailable: the sheet is the same height as on a draft.
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(2);
  });

  // The gate is `status !== "draft"`, not `status === "sent"`. A single
  // non-draft fixture cannot tell those two apart.
  it("disables Delete on a SCHEDULED campaign too", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-scheduled");
    expect(getByTestId("action-sheet-item-delete").props.accessibilityState.disabled).toBe(true);
  });

  it("raises no confirm and sends nothing when the disabled Delete is tapped", () => {
    const spy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-sent");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    expect(spy).not.toHaveBeenCalled();
    expect(mockDelete).not.toHaveBeenCalled();
    spy.mockRestore();
  });
});

describe("Campaigns — Delete is confirmed before it fires", () => {
  let alertSpy: jest.SpyInstance;

  beforeEach(() => {
    alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
  });
  afterEach(() => alertSpy.mockRestore());

  it("does not fire the mutation until the confirm is accepted", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-draft");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    expect(alertSpy).toHaveBeenCalledTimes(1);
    expect(mockDelete).not.toHaveBeenCalled();
  });

  it("names the campaign in the confirm and says the delete is permanent", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-draft");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    expect(alertSpy.mock.calls[0][0]).toBe("Delete campaign?");
    expect(alertSpy.mock.calls[0][1]).toContain("Winter warmers");
    expect(alertSpy.mock.calls[0][1]).toContain("permanently deleted");
    expect(alertSpy.mock.calls[0][1]).toContain("cannot be undone");
  });

  // Cancelling must be INERT — not a handler that could later grow a body.
  it("offers a Cancel with no handler at all", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-draft");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    const buttons = alertSpy.mock.calls[0][2] as { text: string; onPress?: () => void }[];
    const cancel = buttons.find((b) => b.text === "Cancel");
    expect(cancel).toBeDefined();
    expect(cancel?.onPress).toBeUndefined();
    expect(mockDelete).not.toHaveBeenCalled();
  });

  it("sends the delete with just the campaign id once accepted", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-draft");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy);
    expect(mockDelete).toHaveBeenCalledTimes(1);
    expect(mockDelete.mock.calls[0][0]).toBe("cam-draft");
  });

  // NO OPTIMISTIC HIDE. `useDeleteCampaign` invalidates ["campaigns"], which
  // prefix-matches this screen's own key, so the refetch is authoritative
  // about the list's contents.
  it("leaves the row on screen until the refetch removes it", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    longPress(getByTestId, "cam-draft");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy);
    expect(getByTestId("campaign-row-cam-draft")).toBeTruthy();
  });
});

describe("Campaigns — no swipe", () => {
  /**
   * Delete is legal on drafts only, unrecoverable, and 404s on a second call.
   * Nothing on this row belongs behind a one-thumb gesture. Asserted rather
   * than assumed: a later task adding a `SwipeRow` here has to justify it.
   */
  it("mounts no SwipeRow on any row", () => {
    const { UNSAFE_root } = render(<CampaignsScreen />);
    expect(swipeRows(UNSAFE_root)).toHaveLength(0);
  });
});

describe("Campaigns — in-flight rows", () => {
  let alertSpy: jest.SpyInstance;
  beforeEach(() => {
    alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
  });
  afterEach(() => alertSpy.mockRestore());

  function fireDelete(root: ReturnType<typeof render>) {
    longPress(root.getByTestId, "cam-draft");
    fireEvent.press(root.getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy, alertSpy.mock.calls.length - 1);
  }

  it("suppresses long-press while that campaign's own request is open", () => {
    const root = render(<CampaignsScreen />);
    fireDelete(root);
    expect(campaignRow(root.UNSAFE_root, "cam-draft")?.props.onLongPress).toBeUndefined();
    // A DIFFERENT row is untouched — the guard is per row.
    expect(campaignRow(root.UNSAFE_root, "cam-sent")?.props.onLongPress).toBeDefined();
  });

  it("re-arms the row once the mutation settles", () => {
    const root = render(<CampaignsScreen />);
    fireDelete(root);
    act(() => (mockDelete.mock.calls[0][1].onSuccess as () => void)());
    expect(campaignRow(root.UNSAFE_root, "cam-draft")?.props.onLongPress).toBeDefined();
  });

  it("releases the row on failure too, so a failed delete is retryable", () => {
    const root = render(<CampaignsScreen />);
    fireDelete(root);
    act(() => (mockDelete.mock.calls[0][1].onError as (e?: unknown) => void)(new Error("x")));
    expect(campaignRow(root.UNSAFE_root, "cam-draft")?.props.onLongPress).toBeDefined();
  });

  // `settleCallbacks` owns the haptics — the screen must not fire its own, or
  // every action buzzes twice.
  it("fires the success and failure haptics exactly once each", () => {
    const root = render(<CampaignsScreen />);
    fireDelete(root);
    act(() => (mockDelete.mock.calls[0][1].onSuccess as () => void)());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalledTimes(1);

    fireDelete(root);
    act(() => (mockDelete.mock.calls[1][1].onError as (e?: unknown) => void)(new Error("x")));
    expect(adminHaptics.actionFailed).toHaveBeenCalledTimes(1);
  });
});

/**
 * Task 14's surface, on the screen whose failure is otherwise invisible.
 *
 * A refused delete leaves the row exactly where it was — the same pixels a
 * successful delete shows until the refetch lands. Without a message the only
 * difference between "gone in a moment" and "the server said no" is a haptic.
 */
describe("Campaigns — surfacing a failed delete", () => {
  let alertSpy: jest.SpyInstance;
  beforeEach(() => {
    alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
  });
  afterEach(() => alertSpy.mockRestore());

  function apiError(status: number, code: string, message: string): Error {
    const err = new Error(message) as Error & { status: number; code: string };
    err.name = "ApiError";
    err.status = status;
    err.code = code;
    return err;
  }

  function failDelete(root: ReturnType<typeof render>, error: unknown) {
    longPress(root.getByTestId, "cam-draft");
    fireEvent.press(root.getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy, alertSpy.mock.calls.length - 1);
    act(() => (mockDelete.mock.calls.at(-1)?.[1].onError as (e: unknown) => void)(error));
  }

  it("says nothing until something fails", () => {
    const root = render(<CampaignsScreen />);
    longPress(root.getByTestId, "cam-draft");
    fireEvent.press(root.getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy);
    expect(root.queryByTestId("action-failure-notice")).toBeNull();
  });

  it("names what the merchant tried", () => {
    const root = render(<CampaignsScreen />);
    failDelete(root, new TypeError("Network request failed"));
    expect(root.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't delete this campaign",
    );
  });

  // The race the status gate cannot close: a campaign sent from the web
  // between this list's last refetch and the merchant's tap.
  it("passes the server's own reason through for a 409 campaign_not_draft", () => {
    const root = render(<CampaignsScreen />);
    failDelete(
      root,
      apiError(409, "campaign_not_draft", "only draft campaigns can be deleted"),
    );
    // The WHOLE sentence, not a substring: "verbatim" is the property under
    // test, and a substring match would pass against copy that wrapped the
    // server's words in invented framing.
    expect(root.getByTestId("action-failure-detail")).toHaveTextContent(
      "only draft campaigns can be deleted.",
    );
  });

  it("distinguishes an unreachable server from a refusal", () => {
    const root = render(<CampaignsScreen />);
    failDelete(root, new TypeError("Network request failed"));
    expect(root.getByTestId("action-failure-detail")).toHaveTextContent(/reach the server/i);
  });

  it("clears itself on dismiss", () => {
    const root = render(<CampaignsScreen />);
    failDelete(root, new Error("x"));
    fireEvent.press(root.getByTestId("action-failure-dismiss"));
    expect(root.queryByTestId("action-failure-notice")).toBeNull();
  });
});

describe("Campaigns — list states", () => {
  it("sends no status for All and the raw key for every other chip", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    expect(mockListParams[0]).toBeUndefined();
    fireEvent.press(getByTestId("filter-chip-sent-target"));
    expect(mockListParams[mockListParams.length - 1]).toEqual({ status: "sent" });
  });

  it("opens the campaign on tap", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    fireEvent.press(getByTestId("campaign-row-cam-sent"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/campaigns/cam-sent");
  });

  it("tells a screen-reader user the row has more actions", () => {
    const { getByTestId } = render(<CampaignsScreen />);
    expect(getByTestId("campaign-row-cam-draft").props.accessibilityHint).toBe(
      "Long press for more actions",
    );
  });

  it("shows a left-aligned empty state when there are no campaigns", () => {
    mockState.campaigns = [];
    const { getByText } = render(<CampaignsScreen />);
    expect(getByText("No campaigns yet")).toBeTruthy();
  });

  it("offers a retry when the list fails to load", () => {
    mockState.campaigns = [];
    mockState.isError = true;
    const { getByText } = render(<CampaignsScreen />);
    fireEvent.press(getByText("Try again"));
    expect(mockRefetch).toHaveBeenCalledTimes(1);
  });
});

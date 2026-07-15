// `@repo/mobile-shared/auth/link` imports `@react-native-firebase/auth` at
// module scope (unlike provider.tsx's lazy `require("./gip")`), so even
// importing just the `LastSignInMethodError` class pulls in the native
// module chain under Jest. Stub it out — mirrors __tests__/link.test.tsx /
// gip.test.tsx. Mock fns are built INSIDE the factory (babel hoists imports
// above outer const/var).
jest.mock("@react-native-firebase/auth", () => {
  const authFn = () => ({ currentUser: null });
  authFn.GoogleAuthProvider = { credential: jest.fn() };
  authFn.AppleAuthProvider = { credential: jest.fn() };
  return { __esModule: true, default: authFn };
});

const mockAuth: Record<string, unknown> = {};
jest.mock("@repo/mobile-shared/auth/provider", () => ({ useAuth: () => mockAuth }));
jest.mock("@/lib/social-auth", () => ({
  configureGoogleSignin: jest.fn(),
  signInWithGoogleNative: jest.fn().mockResolvedValue("gtok"),
  signInWithAppleNative: jest
    .fn()
    .mockResolvedValue({ idToken: "atok", rawNonce: "", fullName: null }),
}));
// `react-native`'s index.js does `require('./Libraries/Alert/Alert').default`
// (see node_modules/react-native/index.js), so the mock module needs a
// `default` key — a plain `{ alert: fn }` export leaves `Alert` undefined.
jest.mock("react-native/Libraries/Alert/Alert", () => ({
  default: {
    alert: jest.fn((_t, _m, buttons) => {
      // auto-press the destructive/confirm button
      const confirm = (buttons ?? []).find((b: { style?: string }) => b.style === "destructive");
      confirm?.onPress?.();
    }),
  },
}));
// `@/components/ui` barrel re-exports BackHeader/SearchField, which import
// icons from lucide-react-native's ESM build (`dist/esm/...mjs`) — not
// covered by jest-expo's default transformIgnorePatterns, so requiring it
// unmocked throws "Unexpected token 'export'". Stub every icon export with a
// no-op component; we don't assert on icon rendering here.
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});
// `@/components/ui`'s `Screen` calls `useSafeAreaInsets()`, which throws
// without a `<SafeAreaProvider>` ancestor. react-native-safe-area-context
// ships an official jest mock for exactly this (no other test in this repo
// renders `Screen` yet, so this hasn't come up before).
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

import { fireEvent, render, waitFor } from "@testing-library/react-native";
import { LastSignInMethodError } from "@repo/mobile-shared/auth/link";
import SecurityScreen from "../app/(tabs)/more/security";

function setAuth(overrides: Record<string, unknown> = {}) {
  Object.keys(mockAuth).forEach((k) => delete mockAuth[k]);
  Object.assign(
    mockAuth,
    {
      linkedProviderIds: jest.fn().mockResolvedValue(["password"]),
      linkGoogleToCurrentUser: jest.fn().mockResolvedValue(undefined),
      linkAppleToCurrentUser: jest.fn().mockResolvedValue(undefined),
      unlinkProvider: jest.fn().mockResolvedValue(undefined),
    },
    overrides,
  );
}

describe("SecurityScreen", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setAuth();
  });

  it("shows a Link action for providers that are not linked", async () => {
    const { getByLabelText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Link Google")).toBeTruthy());
    expect(getByLabelText("Link Apple")).toBeTruthy();
  });

  it("links Google via the native flow then refreshes", async () => {
    const { getByLabelText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Link Google")).toBeTruthy());
    fireEvent.press(getByLabelText("Link Google"));
    await waitFor(() =>
      expect(mockAuth.linkGoogleToCurrentUser).toHaveBeenCalledWith("gtok"),
    );
    await waitFor(() =>
      expect((mockAuth.linkedProviderIds as jest.Mock).mock.calls.length).toBeGreaterThan(1),
    );
  });

  it("links Apple via the native flow (Hide-My-Email path)", async () => {
    const { getByLabelText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Link Apple")).toBeTruthy());
    fireEvent.press(getByLabelText("Link Apple"));
    await waitFor(() =>
      expect(mockAuth.linkAppleToCurrentUser).toHaveBeenCalledWith("atok", ""),
    );
  });

  it("removes a linked provider after confirming", async () => {
    setAuth({ linkedProviderIds: jest.fn().mockResolvedValue(["password", "google.com"]) });
    const { getByLabelText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Remove Google")).toBeTruthy());
    fireEvent.press(getByLabelText("Remove Google"));
    await waitFor(() => expect(mockAuth.unlinkProvider).toHaveBeenCalledWith("google.com"));
  });

  it("shows the guard copy and does not unlink the only method", async () => {
    setAuth({
      linkedProviderIds: jest.fn().mockResolvedValue(["password"]),
      unlinkProvider: jest.fn().mockRejectedValue(new LastSignInMethodError()),
    });
    const { getByLabelText, findByText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Remove Password")).toBeTruthy());
    fireEvent.press(getByLabelText("Remove Password"));
    expect(await findByText(/only sign-in method/i)).toBeTruthy();
  });

  it("maps credential-already-in-use to friendly copy", async () => {
    setAuth({
      linkGoogleToCurrentUser: jest
        .fn()
        .mockRejectedValue(Object.assign(new Error("x"), { code: "auth/credential-already-in-use" })),
    });
    const { getByLabelText, findByText } = render(<SecurityScreen />);
    await waitFor(() => expect(getByLabelText("Link Google")).toBeTruthy());
    fireEvent.press(getByLabelText("Link Google"));
    expect(await findByText(/already linked to a different Mark8ly account/i)).toBeTruthy();
  });
});

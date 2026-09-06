// "Your session ended. Sign in again." must only ever be shown to someone who
// HAD a session.
//
// On a first launch the auth gate has not routed to /login yet, so the mounted
// dashboard fires its queries and every one of them 401s with no token at all.
// That drove onUnauthorized("no-session") → setNotice, and a merchant opening
// the app for the very first time was told their session had ended. Device
// repro: reset the simulator keychain, launch, and the login form comes up
// carrying the notice.
import React from "react";
import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useApiClient } from "@/lib/api-client";
import { useAuthNoticeStore } from "@repo/mobile-shared/stores/auth-notice";
import { zitadelSession } from "@repo/mobile-shared/auth/zitadel-session";

jest.mock("expo-secure-store", () => {
  const mem: Record<string, string> = {};
  return {
    __mem: mem,
    getItemAsync: jest.fn(async (k: string) => mem[k] ?? null),
    setItemAsync: jest.fn(async (k: string, v: string) => {
      mem[k] = v;
    }),
    deleteItemAsync: jest.fn(async (k: string) => {
      delete mem[k];
    }),
  };
});

const mockSignOut = jest.fn(async () => {});
let mockUser: { uid: string; email: string | null; displayName: string | null } | null = null;
let mockRefreshed: string | null = null;

jest.mock("@repo/mobile-shared/auth/provider", () => ({
  useAuth: () => ({
    user: mockUser,
    // GIP getter: null whenever Firebase holds no user, which is exactly the
    // fresh-install case on a GIP build.
    getToken: async () => (mockUser ? "gip-token" : null),
    refreshToken: async () => mockRefreshed,
    signOut: mockSignOut,
  }),
}));

jest.mock("@repo/mobile-shared/config/env", () => ({
  useEnvironment: () => ({ apiBaseUrl: "https://api.mark8ly.test" }),
}));

const mem = (jest.requireMock("expo-secure-store") as { __mem: Record<string, string> }).__mem;

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

async function requestAndSettle() {
  const { result } = renderHook(() => useApiClient(), { wrapper });
  await result.current.getTenant("/stores").catch(() => undefined);
  await waitFor(() => expect(mockSignOut).toHaveBeenCalled());
}

beforeEach(() => {
  for (const k of Object.keys(mem)) delete mem[k];
  mockSignOut.mockClear();
  mockUser = null;
  mockRefreshed = null;
  useAuthNoticeStore.getState().clearNotice();
  globalThis.fetch = jest.fn() as unknown as typeof fetch;
});

afterEach(() => {
  delete process.env.EXPO_PUBLIC_AUTH_PROVIDER;
});

describe("Zitadel build", () => {
  beforeEach(() => {
    process.env.EXPO_PUBLIC_AUTH_PROVIDER = "zitadel";
  });

  it("shows no notice on a first launch, when no session was ever stored", async () => {
    await requestAndSettle();

    expect(useAuthNoticeStore.getState().notice).toBeNull();
  });

  it("still shows the notice when a stored session has expired", async () => {
    // The record survives expiry, which is what makes "your session ended"
    // true here and false above.
    await zitadelSession.save("AT", "RT", -10);

    await requestAndSettle();

    expect(useAuthNoticeStore.getState().notice).toBe("no-session");
  });

  it("always shows access-denied, which is only reachable with a live token", async () => {
    await zitadelSession.save("AT", "RT", 3600);
    mockRefreshed = "fresh";
    globalThis.fetch = jest
      .fn()
      .mockResolvedValue({ status: 401, ok: false, json: async () => ({}) }) as unknown as typeof fetch;

    await requestAndSettle();

    expect(useAuthNoticeStore.getState().notice).toBe("access-denied");
  });
});

describe("GIP build", () => {
  it("shows no notice when Firebase holds no user", async () => {
    await requestAndSettle();

    expect(useAuthNoticeStore.getState().notice).toBeNull();
  });

  it("shows the notice when Firebase still holds the signed-in user", async () => {
    mockUser = { uid: "u1", email: "a@b.test", displayName: null };
    mockRefreshed = null;
    globalThis.fetch = jest
      .fn()
      .mockResolvedValue({ status: 401, ok: false, json: async () => ({}) }) as unknown as typeof fetch;

    await requestAndSettle();

    expect(useAuthNoticeStore.getState().notice).toBe("no-session");
  });
});

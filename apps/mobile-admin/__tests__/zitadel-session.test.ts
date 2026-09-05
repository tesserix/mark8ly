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

const mem = (jest.requireMock("expo-secure-store") as { __mem: Record<string, string> }).__mem;
beforeEach(() => {
  for (const k of Object.keys(mem)) delete mem[k];
  jest.useRealTimers();
});

it("round-trips a session", async () => {
  await zitadelSession.save("AT", "RT", 3600);
  const s = await zitadelSession.read();
  expect(s?.accessToken).toBe("AT");
  expect(s?.refreshToken).toBe("RT");
  expect(await zitadelSession.accessTokenIfFresh()).toBe("AT");
});

// The server sends seconds-from-now. Storing that verbatim would make it
// meaningless after a restart — an hour-old "3600" would still read fresh.
it("stores an absolute expiry, not a relative one", async () => {
  const before = Date.now();
  await zitadelSession.save("AT", "RT", 3600);
  const s = await zitadelSession.read();
  expect(s!.expiresAt).toBeGreaterThanOrEqual(before + 3600 * 1000);
});

it("treats an expired token as absent", async () => {
  await zitadelSession.save("AT", "RT", -10);
  expect(await zitadelSession.accessTokenIfFresh()).toBeNull();
  // read() still returns it: the refresh path needs the refresh token even
  // once the access token has lapsed.
  expect((await zitadelSession.read())?.refreshToken).toBe("RT");
});

// A token expiring seconds from now would lapse mid-request and surface as
// a 401 the client reads as "signed out".
it("treats a token inside the skew window as stale", async () => {
  await zitadelSession.save("AT", "RT", 30);
  expect(await zitadelSession.accessTokenIfFresh()).toBeNull();
});

// Corrupt state must fail toward re-authentication, never toward trusting a
// token forever.
it("treats an unparseable expiry as elapsed", async () => {
  await zitadelSession.save("AT", "RT", 3600);
  mem["mark8ly_zitadel_expires_at"] = "not-a-number";
  expect(await zitadelSession.accessTokenIfFresh()).toBeNull();
});

it("clears everything on sign-out", async () => {
  await zitadelSession.save("AT", "RT", 3600);
  await zitadelSession.clear();
  expect(await zitadelSession.read()).toBeNull();
});

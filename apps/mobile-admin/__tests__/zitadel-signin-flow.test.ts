import { createZitadelSignIn } from "@repo/mobile-shared/auth/zitadel-signin";
import { isZitadelProvider } from "@/lib/auth-provider";
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

function respond(body: unknown, status = 200) {
  global.fetch = jest.fn(() =>
    Promise.resolve({
      ok: status < 300,
      status,
      json: () => Promise.resolve(body),
    } as unknown as Response),
  ) as unknown as typeof fetch;
}

beforeEach(() => {
  for (const k of Object.keys(mem)) delete mem[k];
});

// The flag must default OFF so an unmodified build keeps using GIP.
it("uses the GIP provider unless explicitly opted in", () => {
  delete process.env.EXPO_PUBLIC_AUTH_PROVIDER;
  expect(isZitadelProvider()).toBe(false);
  process.env.EXPO_PUBLIC_AUTH_PROVIDER = "zitadel";
  expect(isZitadelProvider()).toBe(true);
  delete process.env.EXPO_PUBLIC_AUTH_PROVIDER;
});

it("persists tokens and the tenant on a completed sign-in", async () => {
  respond({
    data: {
      uid: "u1", email: "a@b.test", tenant_id: "t-1",
      access_token: "AT", refresh_token: "RT", expires_in: 3600,
    },
  });
  const setTenantId = jest.fn();
  const flow = createZitadelSignIn("https://api.mark8ly.com");

  const out = await flow.signIn("a@b.test", "pw", setTenantId);

  expect(out.kind).toBe("signed_in");
  expect(await zitadelSession.accessTokenIfFresh()).toBe("AT");
  // The TENANT id, from the login response — never a store id.
  expect(setTenantId).toHaveBeenCalledWith("t-1");
});

// The common first login. Nothing may be persisted yet: a stored tenant
// with no token leaves the app sending X-Acting-Tenant-Id unauthenticated,
// which reads as a server fault rather than an unfinished login.
it("persists nothing while an OTP challenge is outstanding", async () => {
  respond({
    data: {
      email: "a@b.test", tenant_id: "t-1",
      email_otp_required: true, pending_token: "sealed",
    },
  });
  const setTenantId = jest.fn();
  const flow = createZitadelSignIn("https://api.mark8ly.com");

  const out = await flow.signIn("a@b.test", "pw", setTenantId);

  expect(out.kind).toBe("otp");
  expect(out.pendingToken).toBe("sealed");
  expect(await zitadelSession.read()).toBeNull();
  expect(setTenantId).not.toHaveBeenCalled();
});

it("persists on OTP completion", async () => {
  respond({
    data: {
      uid: "u1", email: "a@b.test", tenant_id: "t-1",
      access_token: "AT2", refresh_token: "RT2", expires_in: 3600,
    },
  });
  const setTenantId = jest.fn();
  const flow = createZitadelSignIn("https://api.mark8ly.com");

  const out = await flow.verifyOtp("sealed", "123456", setTenantId);

  expect(out.kind).toBe("signed_in");
  expect(await zitadelSession.accessTokenIfFresh()).toBe("AT2");
  expect(setTenantId).toHaveBeenCalledWith("t-1");
});

it("clears the session on sign-out", async () => {
  await zitadelSession.save("AT", "RT", 3600);
  await createZitadelSignIn("https://api.mark8ly.com").signOut();
  expect(await zitadelSession.read()).toBeNull();
});

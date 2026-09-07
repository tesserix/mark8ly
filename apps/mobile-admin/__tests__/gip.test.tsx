// Regression guard for the GIP tenant-scoping bug: @react-native-firebase
// v22+ makes `tenantId` a READ-ONLY getter, so `firebaseAuth.tenantId = x`
// throws "Proxy set returned false for property 'tenantId'". createGIPAuth
// must scope the tenant via setTenantId() and AWAIT it before any sign-in.
//
// The mock below defines `tenantId` with a throwing setter to emulate the
// real SDK — so a regression back to direct assignment fails "without
// throwing". Mock fns are built INSIDE the jest.mock factory (babel hoists
// ESM imports above outer const/var, so outer-scope refs would be undefined)
// and read back off the imported `auth` default export.

jest.mock("@react-native-firebase/auth", () => {
  const instance = {
    setTenantId: jest.fn().mockResolvedValue(undefined),
    signInWithEmailAndPassword: jest.fn().mockResolvedValue({ user: { uid: "u1" } }),
    signOut: jest.fn().mockResolvedValue(undefined),
    onAuthStateChanged: jest.fn(),
    sendPasswordResetEmail: jest.fn(),
    currentUser: null as unknown,
  };
  Object.defineProperty(instance, "tenantId", {
    configurable: true,
    get() {
      return null;
    },
    set() {
      throw new TypeError("Proxy set returned false for property 'tenantId'");
    },
  });
  return { __esModule: true, getAuth: () => instance };
});

jest.mock("@repo/mobile-shared/auth/social-credentials", () => ({
  signInWithGoogleCredential: jest.fn().mockResolvedValue({ status: "signed-in" }),
  signInWithAppleCredential: jest.fn().mockResolvedValue({ status: "signed-in" }),
}));

jest.mock("@repo/mobile-shared/auth/link", () => ({
  completeLinkWithPassword: jest.fn().mockResolvedValue(undefined),
  completeLinkWithGoogle: jest.fn().mockResolvedValue(undefined),
  completeLinkWithApple: jest.fn().mockResolvedValue(undefined),
  existingSignInMethods: jest.fn().mockResolvedValue(["password"]),
  linkedProviderIds: jest.fn().mockResolvedValue(["password"]),
  linkGoogleToCurrentUser: jest.fn().mockResolvedValue(undefined),
  linkAppleToCurrentUser: jest.fn().mockResolvedValue(undefined),
  unlinkProvider: jest.fn().mockResolvedValue(undefined),
}));

import { getAuth } from "@react-native-firebase/auth";
import {
  signInWithGoogleCredential,
  signInWithAppleCredential,
} from "@repo/mobile-shared/auth/social-credentials";
import { completeLinkWithPassword } from "@repo/mobile-shared/auth/link";
import { createGIPAuth } from "@repo/mobile-shared/auth/gip";

interface MockedAuthInstance {
  setTenantId: jest.Mock;
  signInWithEmailAndPassword: jest.Mock;
  signOut: jest.Mock;
  currentUser: unknown;
}

const instance = (getAuth as unknown as () => MockedAuthInstance)();
const mockGoogleCredential = signInWithGoogleCredential as jest.Mock;
const mockAppleCredential = signInWithAppleCredential as jest.Mock;

describe("createGIPAuth tenant scoping", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    instance.setTenantId.mockResolvedValue(undefined);
  });

  it("scopes the GIP tenant via setTenantId (not direct assignment) without throwing", () => {
    expect(() => createGIPAuth({ tenantId: "MP-Internal-e986p" })).not.toThrow();
    expect(instance.setTenantId).toHaveBeenCalledWith("MP-Internal-e986p");
  });

  it("awaits the tenant before email/password sign-in", async () => {
    const gip = createGIPAuth({ tenantId: "T1" });
    await gip.signIn("merchant@store.com", "pw");
    expect(instance.setTenantId).toHaveBeenCalledWith("T1");
    expect(instance.signInWithEmailAndPassword).toHaveBeenCalledWith(
      "merchant@store.com",
      "pw",
    );
  });

  it("does not call the sign-in native method until setTenantId resolves", async () => {
    let resolveTenant!: () => void;
    instance.setTenantId.mockReturnValueOnce(
      new Promise<void>((resolve) => {
        resolveTenant = resolve;
      }),
    );
    const gip = createGIPAuth({ tenantId: "T2" });
    const pending = gip.signIn("a@b.com", "pw");
    // Flush microtasks — if sign-in didn't await the tenant it would fire now.
    await Promise.resolve();
    await Promise.resolve();
    expect(instance.signInWithEmailAndPassword).not.toHaveBeenCalled();
    resolveTenant();
    await pending;
    expect(instance.signInWithEmailAndPassword).toHaveBeenCalledWith("a@b.com", "pw");
  });

  it("awaits the tenant before Google credential sign-in", async () => {
    const gip = createGIPAuth({ tenantId: "T3" });
    await gip.signInWithGoogle("gtok");
    expect(instance.setTenantId).toHaveBeenCalledWith("T3");
    expect(mockGoogleCredential).toHaveBeenCalledWith("gtok", undefined);
  });

  it("awaits the tenant before Apple credential sign-in", async () => {
    const gip = createGIPAuth({ tenantId: "T4" });
    await gip.signInWithApple("atok", "nonce", null);
    expect(instance.setTenantId).toHaveBeenCalledWith("T4");
    expect(mockAppleCredential).toHaveBeenCalledWith("atok", "nonce", null);
  });

  it("awaits the tenant before completing a password link", async () => {
    const gip = createGIPAuth({ tenantId: "T5" });
    const pending = { provider: "google", idToken: "p" } as never;
    await gip.completeLinkWithPassword("merchant@store.com", "pw", pending);
    expect(instance.setTenantId).toHaveBeenCalledWith("T5");
    expect(completeLinkWithPassword as jest.Mock).toHaveBeenCalledWith(
      "merchant@store.com",
      "pw",
      pending,
    );
  });
});

// Signing out when nobody is signed in must be a NO-OP, not a rejection.
//
// On a Zitadel build the Firebase SDK never has a current user, so
// `firebaseAuth.signOut()` rejects with `auth/no-current-user`. The provider
// clears the tokens that actually hold the session BEFORE calling this, so the
// person really is signed out — the rejection was pure noise: a red-box console
// error on the device in dev, an unhandled promise rejection in production, and
// worst of all it made every single sign-out look broken, which would mask a
// real one. Observed on an iPhone 17 Pro Max simulator, 2026-09-07.
describe("createGIPAuth sign-out is idempotent", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    instance.setTenantId.mockResolvedValue(undefined);
  });

  it("resolves without calling Firebase when no user is signed in", async () => {
    instance.currentUser = null;
    const gip = createGIPAuth({ tenantId: "T6" });
    await expect(gip.signOut()).resolves.toBeUndefined();
    expect(instance.signOut).not.toHaveBeenCalled();
  });

  it("swallows auth/no-current-user when the user disappears mid-call", async () => {
    instance.currentUser = { uid: "u1" };
    instance.signOut.mockRejectedValueOnce(
      Object.assign(new Error("No user currently signed in."), {
        code: "auth/no-current-user",
      }),
    );
    const gip = createGIPAuth({ tenantId: "T7" });
    await expect(gip.signOut()).resolves.toBeUndefined();
    expect(instance.signOut).toHaveBeenCalled();
  });

  it("still signs out normally when a user IS signed in", async () => {
    instance.currentUser = { uid: "u1" };
    instance.signOut.mockResolvedValueOnce(undefined);
    const gip = createGIPAuth({ tenantId: "T8" });
    await gip.signOut();
    expect(instance.signOut).toHaveBeenCalledTimes(1);
  });

  it("propagates a genuine sign-out failure", async () => {
    instance.currentUser = { uid: "u1" };
    instance.signOut.mockRejectedValueOnce(
      Object.assign(new Error("network request failed"), {
        code: "auth/network-request-failed",
      }),
    );
    const gip = createGIPAuth({ tenantId: "T9" });
    await expect(gip.signOut()).rejects.toThrow("network request failed");
  });
});

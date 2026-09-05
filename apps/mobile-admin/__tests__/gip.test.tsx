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

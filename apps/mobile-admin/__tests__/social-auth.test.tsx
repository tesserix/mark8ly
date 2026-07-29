jest.mock("@react-native-google-signin/google-signin", () => ({
  GoogleSignin: {
    configure: jest.fn(),
    hasPlayServices: jest.fn().mockResolvedValue(true),
    signIn: jest.fn(),
  },
}));

jest.mock("expo-apple-authentication", () => ({
  signInAsync: jest.fn(),
  AppleAuthenticationScope: { FULL_NAME: 1, EMAIL: 0 },
}));

import { Platform } from "react-native";
import { GoogleSignin } from "@react-native-google-signin/google-signin";
import * as AppleAuthentication from "expo-apple-authentication";
import { AuthCancelledError } from "@repo/mobile-shared/auth/errors";
import { signInWithAppleNative, signInWithGoogleNative } from "@/lib/social-auth";

const mockGoogleSignIn = GoogleSignin.signIn as jest.Mock;
const mockAppleSignIn = AppleAuthentication.signInAsync as jest.Mock;

const ORIGINAL_PLATFORM_OS = Platform.OS;
const ORIGINAL_ENV = { ...process.env };

beforeEach(() => jest.clearAllMocks());

afterEach(() => {
  Platform.OS = ORIGINAL_PLATFORM_OS;
  process.env = { ...ORIGINAL_ENV };
});

// configureGoogleSignin() caches its result in a module-scoped `configured`
// flag, so each case here needs its own fresh module instance — otherwise
// only the first case in the block would ever observe a real configure()
// call or throw; every case after it would silently short-circuit on the
// `if (configured) return;` guard and the assertion would pass for the wrong
// reason.
function loadFreshSocialAuth(): {
  configureGoogleSignin: () => void;
  googleConfigureMock: jest.Mock;
} {
  let configureGoogleSignin!: () => void;
  let googleConfigureMock!: jest.Mock;
  jest.isolateModules(() => {
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    configureGoogleSignin = require("@/lib/social-auth").configureGoogleSignin;
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    googleConfigureMock = require("@react-native-google-signin/google-signin").GoogleSignin
      .configure;
  });
  return { configureGoogleSignin, googleConfigureMock };
}

describe("configureGoogleSignin", () => {
  it("throws when EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID is missing", () => {
    process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID = "";
    delete process.env.EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID;
    const { configureGoogleSignin } = loadFreshSocialAuth();
    expect(() => configureGoogleSignin()).toThrow(/EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID/);
  });

  it("on Android, throws when EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID is missing — A3 guard", () => {
    // Without a real Android OAuth client (A1), GoogleSignin.configure()
    // would otherwise succeed and let signIn() reach Play Services, which
    // fails with an opaque DEVELOPER_ERROR instead of a clear message.
    Platform.OS = "android";
    process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID = "web-client-id";
    delete process.env.EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID;
    const { configureGoogleSignin, googleConfigureMock } = loadFreshSocialAuth();
    expect(() => configureGoogleSignin()).toThrow(/EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID/);
    expect(googleConfigureMock).not.toHaveBeenCalled();
  });

  it("on Android, configures successfully once EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID is set", () => {
    Platform.OS = "android";
    process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID = "web-client-id";
    process.env.EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID = "android-client-id";
    const { configureGoogleSignin, googleConfigureMock } = loadFreshSocialAuth();
    expect(() => configureGoogleSignin()).not.toThrow();
    expect(googleConfigureMock).toHaveBeenCalledTimes(1);
  });

  it("on iOS, never requires EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID", () => {
    Platform.OS = "ios";
    process.env.EXPO_PUBLIC_GOOGLE_WEB_CLIENT_ID = "web-client-id";
    delete process.env.EXPO_PUBLIC_GOOGLE_ANDROID_CLIENT_ID;
    const { configureGoogleSignin, googleConfigureMock } = loadFreshSocialAuth();
    expect(() => configureGoogleSignin()).not.toThrow();
    expect(googleConfigureMock).toHaveBeenCalledTimes(1);
  });
});

describe("signInWithGoogleNative", () => {
  it("throws AuthCancelledError when the SDK RESOLVES with type:cancelled", async () => {
    // The SDK does not reject on cancel — it resolves with this shape.
    mockGoogleSignIn.mockResolvedValue({ type: "cancelled", data: null });
    await expect(signInWithGoogleNative()).rejects.toBeInstanceOf(AuthCancelledError);
  });

  it("does NOT report a cancel as a missing-token failure", async () => {
    mockGoogleSignIn.mockResolvedValue({ type: "cancelled", data: null });
    await expect(signInWithGoogleNative()).rejects.not.toThrow(/no ID token/i);
  });

  it("returns the idToken on success", async () => {
    mockGoogleSignIn.mockResolvedValue({ type: "success", data: { idToken: "gtok" } });
    await expect(signInWithGoogleNative()).resolves.toBe("gtok");
  });

  it("still throws when a non-cancelled response carries no idToken", async () => {
    mockGoogleSignIn.mockResolvedValue({ type: "success", data: { idToken: null } });
    await expect(signInWithGoogleNative()).rejects.toThrow(/no ID token/i);
  });
});

describe("signInWithAppleNative", () => {
  it("turns ERR_REQUEST_CANCELED into AuthCancelledError", async () => {
    mockAppleSignIn.mockRejectedValue(
      Object.assign(new Error("The user canceled"), { code: "ERR_REQUEST_CANCELED" }),
    );
    await expect(signInWithAppleNative()).rejects.toBeInstanceOf(AuthCancelledError);
  });

  it("leaves other Apple errors untouched", async () => {
    const original = Object.assign(new Error("RequestUnknownException"), {
      code: "ERR_REQUEST_UNKNOWN",
    });
    mockAppleSignIn.mockRejectedValue(original);
    await expect(signInWithAppleNative()).rejects.toBe(original);
  });

  it("returns the identity token on success", async () => {
    mockAppleSignIn.mockResolvedValue({ identityToken: "atok", fullName: null });
    await expect(signInWithAppleNative()).resolves.toEqual({
      idToken: "atok",
      rawNonce: "",
      fullName: null,
    });
  });
});

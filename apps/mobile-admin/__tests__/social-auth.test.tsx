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

import { GoogleSignin } from "@react-native-google-signin/google-signin";
import * as AppleAuthentication from "expo-apple-authentication";
import { AuthCancelledError } from "@repo/mobile-shared/auth/errors";
import { signInWithAppleNative, signInWithGoogleNative } from "@/lib/social-auth";

const mockGoogleSignIn = GoogleSignin.signIn as jest.Mock;
const mockAppleSignIn = AppleAuthentication.signInAsync as jest.Mock;

beforeEach(() => jest.clearAllMocks());

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

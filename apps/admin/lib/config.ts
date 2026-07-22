// Runtime config for the admin app.
//
// Two flavors:
//   - server-only: PLATFORM_API_URL, AUTH_BFF_URL (server actions / middleware)
//   - public:      NEXT_PUBLIC_GIP_*, NEXT_PUBLIC_GOOGLE_CLIENT_ID (browser)

export const config = {
  platformApiUrl: process.env.PLATFORM_API_URL ?? "http://localhost:8086",
  authBffUrl: process.env.AUTH_BFF_URL ?? "http://localhost:8087",
} as const;

export const publicConfig = {
  gipProjectId: process.env.NEXT_PUBLIC_GIP_PROJECT_ID ?? "",
  gipTenantId: process.env.NEXT_PUBLIC_GIP_TENANT_ID ?? "",
  gipApiKey: process.env.NEXT_PUBLIC_GIP_API_KEY ?? "",
  googleClientId: process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID ?? "",
  // Apple *Services ID* (e.g. "com.mark8ly.admin.web") — NOT the iOS bundle
  // ID. Apple treats native and web as separate clients: the native app uses
  // a bundle ID, the web flow needs a Services ID registered with a return
  // URL. Empty until that Services ID exists in the Apple Developer account
  // AND its client secret is configured on the GIP apple.com provider; the
  // sign-in button stays hidden until then, because a button that 400s at
  // Apple is worse than no button.
  appleServicesId: process.env.NEXT_PUBLIC_APPLE_SERVICES_ID ?? "",
} as const;

/**
 * Web Sign in with Apple is deliberately DISABLED (2026-07-22). The full
 * chain (Services ID com.mark8ly.admin.web, domains + return URLs, GIP
 * apple.com provider with code-flow key) is configured and the consent
 * screen renders, but Apple's popup flow dies at the final verification
 * step ("Failed to verify your identity") on the canonical login origin.
 * Mobile Apple sign-in is unaffected and stays live. To re-attempt web,
 * restore `Boolean(publicConfig.appleServicesId)` — all plumbing
 * (apple-js.ts, SignInForm handler) is intact behind this flag.
 */
export const appleSignInEnabled = false;

// Runtime config sourced from env vars.
//
// Two flavors:
//   - server-only:   PLATFORM_API_URL, AUTH_BFF_URL  (used in server actions / server components)
//   - public client: NEXT_PUBLIC_GIP_*               (read directly in browser code)
//
// The NEXT_PUBLIC_GIP_* values are intentionally exposed to the browser:
//   - GIP project ID is public (visible in any Firebase console URL)
//   - GIP tenant ID is public
//   - GIP Web API key is the "client API key" — Google designed it for browser use,
//     it identifies the project but does not authorize any privileged operation

export const config = {
  /** Server-side platform-api base URL. Only used from server actions / server components. */
  platformApiUrl: process.env.PLATFORM_API_URL ?? "http://localhost:8086",
  /** Server-side auth-bff base URL. Only used from server actions / server components. */
  authBffUrl: process.env.AUTH_BFF_URL ?? "http://localhost:8087",
} as const;

export const publicConfig = {
  /** GIP / Firebase project ID. Browser-exposed by design. */
  gipProjectId: process.env.NEXT_PUBLIC_GIP_PROJECT_ID ?? "",
  /** GIP tenant pool for marketplace internal users (admin/onboarding). */
  gipTenantId: process.env.NEXT_PUBLIC_GIP_TENANT_ID ?? "",
  /** GIP tenant pool for marketplace customer (storefront) users.
   *  Used only by the /auth/google trampoline page that bounces
   *  customer sign-ins from per-tenant subdomains. */
  gipCustomerTenantId: process.env.NEXT_PUBLIC_GIP_CUSTOMER_TENANT_ID ?? "",
  /** GIP Web API key. Public by design. */
  gipApiKey: process.env.NEXT_PUBLIC_GIP_API_KEY ?? "",
  /** Google OAuth client ID, used by the GSI library on the /login page
   *  to drive the "Continue with Google" button. Public by design. */
  googleClientId: process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID ?? "",
} as const;

// Runtime config for the admin app.
//
// Two flavors:
//   - server-only: PLATFORM_API_URL, AUTH_BFF_URL (server actions / middleware)
//   - public:      NEXT_PUBLIC_ZITADEL_* (browser)

export const config = {
  platformApiUrl: process.env.PLATFORM_API_URL ?? "http://localhost:8086",
  authBffUrl: process.env.AUTH_BFF_URL ?? "http://localhost:8087",
} as const;

export const publicConfig = {
  zitadelIssuer: process.env.NEXT_PUBLIC_ZITADEL_ISSUER ?? "",
  zitadelAdminClientId: process.env.NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID ?? "",
} as const;

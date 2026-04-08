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
} as const;

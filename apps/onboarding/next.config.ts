import path from "node:path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  // Tell Next.js where the workspace root is so the standalone bundle
  // includes the right node_modules for monorepo builds inside Docker.
  outputFileTracingRoot: path.join(__dirname, "../.."),
  reactStrictMode: true,
  // Same baseline security headers as admin + storefront. The onboarding
  // app collects merchant-signup PII (emails, addresses, tax IDs) and is
  // framed against the marketing domain — it needs identical protections.
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          {
            key: "Strict-Transport-Security",
            value: "max-age=31536000; includeSubDomains",
          },
          {
            key: "Content-Security-Policy",
            value: [
              "default-src 'self'",
              // accounts.google.com hosts the GSI client script used by
              // /auth/google (customer Google sign-in trampoline).
              "script-src 'self' 'unsafe-inline' 'unsafe-eval' https://accounts.google.com/gsi/client",
              "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style",
              "img-src 'self' data: blob: https:",
              "font-src 'self' data:",
              "connect-src 'self' https: wss:",
              "frame-ancestors 'self'",
              // GSI renders the button + One-Tap UI inside an iframe served
              // from accounts.google.com/gsi/.
              "frame-src 'self' https://accounts.google.com/gsi/",
              "object-src 'none'",
              "base-uri 'self'",
              "form-action 'self'",
            ].join("; "),
          },
          { key: "Permissions-Policy", value: "geolocation=(), microphone=(), camera=()" },
        ],
      },
    ];
  },
};

export default nextConfig;

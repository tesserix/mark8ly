import path from "node:path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  // Tell Next.js where the workspace root is so the standalone bundle
  // includes the right node_modules for monorepo builds inside Docker.
  outputFileTracingRoot: path.join(__dirname, "../.."),
  reactStrictMode: true,
  // @tesserix/otto-widget ships raw TypeScript source (no dist/) — Next.js
  // must compile it like first-party app code instead of trying to resolve
  // a pre-built CJS/ESM entrypoint.
  transpilePackages: ["@tesserix/otto-widget"],
  images: {
    // Remote hosts the storefront is allowed to load product media from.
    // GCS (production), local fake-gcs-server (dev), and localhost for
    // any quick smoke-test blob server.
    remotePatterns: [
      { protocol: "https", hostname: "storage.googleapis.com" },
      { protocol: "https", hostname: "*.storage.googleapis.com" },
      { protocol: "https", hostname: "cdn.mark8ly.com" },
      { protocol: "https", hostname: "images.unsplash.com" },
      { protocol: "http", hostname: "localhost" },
      { protocol: "http", hostname: "fake-gcs-server" },
    ],
  },
  headers: async () => [
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
          // 'unsafe-inline' for style-src is required because merchants
          // inject branded CSS via <style> (see sanitizeCss in
          // app/layout.tsx). 'unsafe-inline' for script-src covers
          // Next.js's hydration JSON + the JSON-LD <script> blocks; we
          // gate this with strict object-src 'none' + base-uri 'self'
          // so injection still can't pivot to plugin/data execution.
          value: [
            "default-src 'self'",
            // Storefronts trampoline to mark8ly.com/auth/google for customer
            // Google sign-in, so GSI is not loaded here directly. The
            // allowlist is kept in sync with admin + onboarding so any
            // future inline use (e.g. one-tap on storefront) is unblocked.
            "script-src 'self' 'unsafe-inline' 'unsafe-eval' https://accounts.google.com/gsi/client",
            "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style",
            "img-src 'self' data: blob: https:",
            "font-src 'self' data:",
            "connect-src 'self' https: wss:",
            "frame-ancestors 'self'",
            "frame-src 'self' https://accounts.google.com/gsi/",
            "object-src 'none'",
            "base-uri 'self'",
            "form-action 'self'",
          ].join("; "),
        },
        { key: "Permissions-Policy", value: "geolocation=(), microphone=(), camera=()" },
      ],
    },
  ],
};

export default nextConfig;

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
    remotePatterns: [
      { protocol: "https", hostname: "storage.googleapis.com" },
      { protocol: "https", hostname: "*.storage.googleapis.com" },
      { protocol: "https", hostname: "cdn.mark8ly.com" },
      { protocol: "http", hostname: "localhost" },
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
          value: [
            "default-src 'self'",
            // accounts.google.com hosts the GSI client script used by
            // admin's Google sign-in (SignInForm → getGoogleCredential).
            // appleid.cdn-apple.com hosts the Sign in with Apple JS SDK
            // (SignInForm → getAppleCredential, lib/gip/apple-js.ts) — without
            // it the script tag is blocked and Apple sign-in fails with
            // "apple sdk load failed".
            "script-src 'self' 'unsafe-inline' 'unsafe-eval' https://accounts.google.com/gsi/client https://appleid.cdn-apple.com",
            "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style",
            "img-src 'self' data: blob: https:",
            "font-src 'self' data:",
            "connect-src 'self' https: wss:",
            "frame-ancestors 'self'",
            // GSI renders the button + One-Tap UI inside an iframe served
            // from accounts.google.com/gsi/. Apple's JS SDK injects an iframe
            // from appleid.apple.com to orchestrate the sign-in popup.
            "frame-src 'self' https://accounts.google.com/gsi/ https://appleid.apple.com",
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

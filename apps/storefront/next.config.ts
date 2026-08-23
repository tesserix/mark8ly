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
        // Content-Security-Policy is issued by middleware, which is the
        // only place a per-request nonce can be minted. Two CSP headers
        // would be intersected, so it must not also live here.
        { key: "Permissions-Policy", value: "geolocation=(), microphone=(), camera=()" },
      ],
    },
  ],
};

export default nextConfig;

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
            // Razorpay is allowlisted by wildcard, per their own documented
            // CSP (docs/payments/payment-gateway/cordova-integration).
            // Enumerating hosts does not work here: PaymentPrompt only
            // names checkout.razorpay.com, but that SDK pulls further
            // scripts at runtime (cdn.razorpay.com risk-detection, and
            // lumberjack.razorpay.com telemetry) which no amount of
            // reading our own source reveals. Each missed host is a
            // silent prod-only breakage, so trust the payment
            // processor's domain rather than guess its subdomains.
            "script-src 'self' 'unsafe-inline' 'unsafe-eval' https://accounts.google.com/gsi/client https://*.razorpay.com",
            "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style",
            "img-src 'self' data: blob: https:",
            "font-src 'self' data:",
            "connect-src 'self' https: wss:",
            "frame-ancestors 'self'",
            // Razorpay renders its checkout modal (and the bank/UPI
            // redirect flows) in iframes from api.razorpay.com — allowing
            // only the script leaves the modal blank. Wildcarded for the
            // same reason as script-src above.
            "frame-src 'self' https://accounts.google.com/gsi/ https://*.razorpay.com",
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

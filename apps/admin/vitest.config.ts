import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: false,
    setupFiles: ["./vitest.setup.ts"],
    include: ["**/*.{test,spec}.{ts,tsx}", "../../packages/ui/src/**/*.test.{ts,tsx}"],
    exclude: ["**/node_modules/**", "**/tests/e2e/**", "**/.next/**"],
    server: {
      deps: {
        inline: ["@tesserix/web"],
      },
    },
    coverage: {
      provider: "v8",
      reporter: ["text", "text-summary"],
      include: [
        "lib/products/**/*.{ts,tsx}",
        "components/products/media/**/*.{ts,tsx}",
        "components/products/options/**/*.{ts,tsx}",
        "components/products/variants/**/*.{ts,tsx}",
        "components/products/form/**/*.{ts,tsx}",
      ],
      exclude: [
        "**/*.test.{ts,tsx}",
        "**/tests/e2e/**",
        "**/node_modules/**",
        "components/products/ProductForm.tsx",
      ],
      thresholds: {
        lines: 80,
        branches: 70,
        functions: 80,
        statements: 80,
      },
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "."),
      // @repo/ui sub-path exports — vitest needs explicit aliases because
      // it does not process package.json "exports" maps at test time.
      "@repo/ui/subscription": path.resolve(__dirname, "../../packages/ui/src/subscription/index.ts"),
      "@repo/ui/brand-bar": path.resolve(__dirname, "../../packages/ui/src/brand-bar.tsx"),
      "@repo/ui/price-display": path.resolve(__dirname, "../../packages/ui/src/price-display.tsx"),
      "@repo/ui/role-badge": path.resolve(__dirname, "../../packages/ui/src/role-badge.tsx"),
      "@repo/ui/app-store-badges": path.resolve(__dirname, "../../packages/ui/src/app-store-badges.tsx"),
      "@repo/ui/status-dot": path.resolve(__dirname, "../../packages/ui/src/status-dot.tsx"),
      "@repo/ui/auth/csrf": path.resolve(__dirname, "../../packages/ui/src/auth/csrf.ts"),
      "@repo/ui/auth/zitadel-totp-code": path.resolve(__dirname, "../../packages/ui/src/auth/zitadel-totp-code.ts"),
      "@repo/ui/auth/link-provider-prompt": path.resolve(__dirname, "../../packages/ui/src/auth/link-provider-prompt.tsx"),
      "@repo/ui/field": path.resolve(__dirname, "../../packages/ui/src/field.tsx"),
      "@repo/ui/google-mark": path.resolve(__dirname, "../../packages/ui/src/google-mark.tsx"),
      "@repo/ui/apple-mark": path.resolve(__dirname, "../../packages/ui/src/apple-mark.tsx"),
      "@repo/ui": path.resolve(__dirname, "../../packages/ui/src/index.ts"),
      react: path.resolve(__dirname, "../../node_modules/react"),
      "react-dom": path.resolve(__dirname, "../../node_modules/react-dom"),
      "react/jsx-runtime": path.resolve(__dirname, "../../node_modules/react/jsx-runtime.js"),
      "react/jsx-dev-runtime": path.resolve(__dirname, "../../node_modules/react/jsx-dev-runtime.js"),
    },
    dedupe: ["react", "react-dom"],
  },
});

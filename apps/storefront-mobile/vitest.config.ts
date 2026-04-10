import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: [
      "**/__tests__/**/*.test.ts",
      "**/__tests__/**/*.test.tsx",
      "**/*.test.ts",
      "**/*.test.tsx",
    ],
    exclude: ["node_modules/**"],
    environment: "node",
  },
});

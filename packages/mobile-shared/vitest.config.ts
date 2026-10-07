import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: [
      "src/**/*.test.ts",
      "src/**/*.test.tsx",
      "**/__tests__/**/*.test.ts",
      "**/__tests__/**/*.test.tsx",
    ],
    exclude: ["node_modules/**"],
    environment: "node",
  },
  resolve: {
    alias: {
      // See test/react-native-stub.ts — vitest cannot transform
      // react-native's Flow source, and useSupportChat imports AppState.
      "react-native": new URL("./test/react-native-stub.ts", import.meta.url)
        .pathname,
    },
  },
});

import path from "node:path";
import { defineConfig } from "vitest/config";

// The app's first test runner (#969). Pure modules only — there is no
// React renderer wired for screens, and the upload and picker code take
// injected dependencies precisely so the logic is testable without one.
export default defineConfig({
  test: {
    include: ["lib/**/*.test.ts", "components/**/*.test.ts"],
    exclude: ["node_modules/**"],
    environment: "node",
  },
  resolve: {
    alias: {
      "@": __dirname,
      // Vitest cannot transform the Expo native modules (they import
      // react-native's Flow source). The modules under test only touch
      // them through injected deps, so the stubs exist to make the
      // imports resolve, not to emulate anything. See test/.
      "expo-image-picker": path.resolve(
        __dirname,
        "test/expo-image-picker-stub.ts",
      ),
      "expo-image-manipulator": path.resolve(
        __dirname,
        "test/expo-image-manipulator-stub.ts",
      ),
      "expo-crypto": path.resolve(__dirname, "test/expo-crypto-stub.ts"),
    },
  },
});

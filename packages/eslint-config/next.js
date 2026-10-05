import js from "@eslint/js";
import { globalIgnores } from "eslint/config";
import eslintConfigPrettier from "eslint-config-prettier";
import tseslint from "typescript-eslint";
import pluginReactHooks from "eslint-plugin-react-hooks";
import pluginReact from "eslint-plugin-react";
import globals from "globals";
import pluginNext from "@next/eslint-plugin-next";
import { config as baseConfig } from "./base.js";

/**
 * A custom ESLint configuration for libraries that use Next.js.
 *
 * @type {import("eslint").Linter.Config[]}
 * */
export const nextJsConfig = [
  ...baseConfig,
  js.configs.recommended,
  eslintConfigPrettier,
  ...tseslint.configs.recommended,
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
  ]),
  {
    // Scope React plugin to JSX/TSX files only + pin React version
    // explicitly so eslint-plugin-react's detectReactVersion doesn't
    // walk the filesystem. Under flat config + typescript-eslint v8
    // the `resolveBasedir` path throws `contextOrFilename.getFilename
    // is not a function`. Pinning the version short-circuits that
    // code path before it runs.
    files: ["**/*.{jsx,tsx}"],
    ...pluginReact.configs.flat.recommended,
    languageOptions: {
      ...pluginReact.configs.flat.recommended.languageOptions,
      globals: {
        ...globals.serviceworker,
      },
    },
    settings: {
      ...(pluginReact.configs.flat.recommended.settings ?? {}),
      react: { version: "19.2" },
    },
  },
  {
    plugins: {
      "@next/next": pluginNext,
    },
    rules: {
      ...pluginNext.configs.recommended.rules,
      ...pluginNext.configs["core-web-vitals"].rules,
    },
  },
  {
    files: ["**/*.{js,jsx,ts,tsx}"],
    plugins: {
      "react-hooks": pluginReactHooks,
    },
    settings: { react: { version: "19.2" } },
    rules: {
      ...pluginReactHooks.configs.recommended.rules,
      // React scope no longer necessary with new JSX transform.
      "react/react-in-jsx-scope": "off",

      // These are reported here but CANNOT fail a build: base.js loads
      // eslint-plugin-only-warn, which downgrades every rule in this
      // config to a warning. Setting them to "error" here would look
      // like a gate and be nothing of the kind.
      //
      // The gate that does fail lives in ./react-hooks-strict.js, which
      // deliberately does not extend the base. See its doc comment for
      // why, and for the bug that prompted it.
      //
      // The files glob above covers .ts as well as .tsx because custom
      // hooks live in plain .ts files.
    },
  },
];

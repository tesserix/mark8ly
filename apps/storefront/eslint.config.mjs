import { nextJsConfig } from "@repo/eslint-config/next-js";

/**
 * Storefront ESLint config — extends the shared @repo Next.js flat
 * config with one storefront-specific guardrail: components/layouts/*
 * may not contain JSX text literals. Storefront layouts are reduced
 * to `hero + SectionsRenderer` (see plan
 * docs/superpowers/plans/2026-04-15-storefront-layout-blocks-refactor.md);
 * any merchant-facing copy must come from `<Layout>.defaults.ts` or
 * the merchant-authored homepage_content blob, never from hard-coded
 * JSX in the layout file itself.
 *
 * @type {import("eslint").Linter.Config[]}
 */
export default [
  ...nextJsConfig,
  {
    ignores: [
      "node_modules/**",
      ".next/**",
      "playwright-report/**",
      "test-results/**",
      "tests/e2e/**/*-snapshots/**",
    ],
  },
  {
    files: ["components/layouts/*.tsx"],
    // shared.tsx and index.tsx host the dispatcher + shared types,
    // not merchant copy; only the per-layout files are restricted.
    ignores: [
      "components/layouts/shared.tsx",
      "components/layouts/index.tsx",
    ],
    rules: {
      "react/jsx-no-literals": [
        "error",
        {
          // Allow nothing; layouts are pure-blocks now. Move any copy
          // into the layout's .defaults.ts recipe.
          noStrings: true,
          allowedStrings: [],
          ignoreProps: true,
        },
      ],
    },
  },
];

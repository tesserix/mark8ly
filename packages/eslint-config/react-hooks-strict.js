import pluginNext from "@next/eslint-plugin-next";
import pluginA11y from "eslint-plugin-jsx-a11y";
import pluginReact from "eslint-plugin-react";
import pluginReactHooks from "eslint-plugin-react-hooks";
import tseslint from "typescript-eslint";

/**
 * The Rules of Hooks, as a gate that can actually fail.
 *
 * # Why this is a separate config and not two lines in next.js
 *
 * The shared base config loads `eslint-plugin-only-warn`, which
 * downgrades EVERY rule in every config that includes it to a warning.
 * That is why `npm run lint` has always reported "0 errors, N warnings"
 * and exited 0: no lint rule anywhere in this monorepo could fail a
 * build, and setting a rule to "error" there does nothing at all.
 *
 * That turned ESLint into decoration. It found a missing `useCallback`
 * dependency in AddToCartButton — the bug that made the whole
 * custom-products feature a no-op in production, with every buyer's
 * uploaded photo silently dropped between the form and the cart
 * (mark8ly#966) — and reported it as warning number 97 of 110. It found
 * the same class of bug quoting shipping rates against a stale parcel
 * weight. Nobody saw either.
 *
 * Removing only-warn outright would turn several hundred pre-existing
 * warnings into errors and break every build at once, so this config
 * deliberately does NOT extend the shared base. It is a small, strict,
 * standalone pass that apps run alongside their normal lint:
 *
 *     "lint": "eslint . && eslint . -c <this>"
 *
 * Everything else keeps its current severity. Only hooks fail.
 *
 * # Why these two rules specifically
 *
 * They are the two whose violations are silent in production. A missing
 * dependency does not crash, does not fail a type check and does not
 * fail a test — it just quietly serves a stale value, which is the
 * hardest kind of bug to find and the easiest to prevent.
 *
 * The file glob covers .ts as well as .tsx: custom hooks live in plain
 * .ts files, and `usePersonalisationPreviews.ts` is one of them.
 *
 * If this goes red, fix the hook. Do not demote the rule — the whole
 * point is that the last time these were warnings, they were ignored.
 *
 * # Inline suppressions are still honoured, for now
 *
 * Turning them off surfaces TEN pre-existing
 * `eslint-disable-next-line react-hooks/exhaustive-deps` comments across
 * the storefront, admin and onboarding — not the three a storefront-only
 * measurement suggested. The storefront's three are fixed (#1000); the
 * rest are in code whose owners should judge them one at a time, and
 * several are URL-pushing effects where naively satisfying the rule
 * creates a re-render loop rather than removing a bug.
 *
 * The bug that prompted all this carried no disable comment, so this
 * gate would have caught it as it stands. Tightening to
 * `noInlineConfig` once those three are resolved is the obvious
 * follow-up.
 */
export const reactHooksStrict = [
  ...tseslint.configs.recommended.map((c) => ({
    ...c,
    // Only the parser matters here; the recommended RULES would drag in
    // the warning noise this config exists to stay clear of.
    rules: {},
  })),
  {
    files: ["**/*.{js,jsx,ts,tsx}"],
    plugins: {
      react: pluginReact,
      "react-hooks": pluginReactHooks,
      // Registered with NO rules enabled. Existing files carry
      // `eslint-disable` comments for these plugins' rules, and an
      // unknown rule inside a disable directive is itself an error — so
      // they have to be loadable here even though this gate runs none of
      // them. Add to this list rather than weakening the gate if a new
      // plugin's disable comments start failing it.
      "@next/next": pluginNext,
      "jsx-a11y": pluginA11y,
    },
    settings: { react: { version: "19.2" } },
    rules: {
      "react-hooks/exhaustive-deps": "error",
      "react-hooks/rules-of-hooks": "error",
    },
  },
  {
    ignores: [
      "node_modules/**",
      ".next/**",
      "dist/**",
      "playwright-report/**",
      "test-results/**",
      "tests/e2e/**",
      "tests/operator/**",
    ],
  },
];

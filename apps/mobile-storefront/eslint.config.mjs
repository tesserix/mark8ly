import { reactHooksStrict } from "@repo/eslint-config/react-hooks-strict";

/**
 * The Rules of Hooks, for mobile.
 *
 * Until this file existed `npm run lint` here did not fail — it died,
 * with "ESLint couldn't find an eslint.config". There is no root flat
 * config either, so `react-hooks/exhaustive-deps` had never run against
 * a single line of this app, and the `eslint-disable-next-line
 * react-hooks/exhaustive-deps` comments in it suppressed nothing.
 *
 * That is not academic. mark8ly#1016 was exactly the bug this rule
 * exists to catch: checkout re-quoted shipping on `selectedAddress?.id`
 * and `checkoutLines.length`, so a buyer who edited their address — or
 * swapped a line without changing the count — was charged for a quote
 * that no longer matched the order.
 *
 * Deliberately the strict hooks gate and nothing else. The shared base
 * config loads `eslint-plugin-only-warn`, which downgrades every rule
 * everywhere to a warning; adopting it here would re-create the state
 * this file is fixing. See packages/eslint-config/react-hooks-strict.js.
 */
export default [
  ...reactHooksStrict,
  {
    // This config enables exactly two rules, so every `eslint-disable`
    // written for any OTHER rule looks unused to it. Reporting those
    // would bury the hook errors under a dozen false positives about
    // directives that are doing their job under the app's own lint.
    linterOptions: { reportUnusedDisableDirectives: "off" },
  },
  {
    ignores: [
      "node_modules/**",
      ".expo/**",
      "android/**",
      "ios/**",
      "dist/**",
      "scripts/**",
      "*.config.js",
    ],
  },
];

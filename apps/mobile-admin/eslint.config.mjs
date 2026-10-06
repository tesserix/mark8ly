import { reactHooksStrict } from "@repo/eslint-config/react-hooks-strict";

/**
 * The Rules of Hooks, for mobile.
 *
 * This app's `lint` script was an echo stub that described the problem
 * rather than solving it: there was no flat config and no eslint
 * dependency, so `eslint .` was an unconditional fatal error. There is
 * no root flat config either, so `react-hooks/exhaustive-deps` had
 * never run here.
 *
 * The sibling app shipped mark8ly#1016 through that gap — checkout
 * re-quoted shipping on an id and a line count rather than on the
 * values that form the request, and silently charged buyers a stale
 * price. Nothing about this app makes it immune to the same class.
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

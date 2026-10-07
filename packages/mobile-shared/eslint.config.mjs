import { reactHooksStrict } from "@repo/eslint-config/react-hooks-strict";

/**
 * The Rules of Hooks, for the shared mobile package.
 *
 * mobile-storefront and mobile-admin were gated in mark8ly#1017, but this
 * package sits outside both of their base paths, so eslint reported
 * "File ignored because outside of base path" and the rule ran against
 * none of it. That is the same gap as the apps before #1018, one level
 * down — and it is not an empty one: useSupportChat.ts carried two
 * exhaustive-deps suppressions, one hiding a self-referential useCallback
 * and one a socket-opening effect.
 *
 * Deliberately the strict hooks gate and nothing else. The shared base
 * config loads eslint-plugin-only-warn, which downgrades every rule
 * everywhere to a warning; adopting it here would re-create the state
 * this file is fixing.
 */
export default [
  ...reactHooksStrict,
  {
    // This config enables exactly two rules, so every `eslint-disable`
    // written for any OTHER rule looks unused to it.
    linterOptions: { reportUnusedDisableDirectives: "off" },
  },
  {
    ignores: ["node_modules/**", "dist/**", "*.config.js"],
  },
];

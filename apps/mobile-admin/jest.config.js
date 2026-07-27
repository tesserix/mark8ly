// Pin `react-native` resolution to this app's local install. The monorepo
// root hoists a different react-native version (used by another app in the
// npm workspace), and jest-expo's transitive @react-native/jest-preset
// resolves `react-native` relative to *its own* location — which lands on
// the hoisted root copy instead of this app's version, breaking native
// module mocks (e.g. "PlatformConstants could not be found"). Overriding
// moduleNameMapper here forces every `react-native` (and subpath) import
// back to apps/mobile-admin/node_modules/react-native.
// Same hoisting issue as react-native above, but for @react-native-firebase:
// packages/mobile-shared has no local node_modules, so its imports of
// @react-native-firebase/auth resolve to the hoisted ROOT copy (pinned to an
// older major version by another app in the workspace) instead of this app's
// local copy. That mismatched, un-mocked module then tries to init native
// modules and blows up under jest. Force both packages back to this app's
// local install so jest.mock("@react-native-firebase/auth", ...) in tests
// actually intercepts the module the source code imports.
// Same class of bug again, but for `react`: `zustand` exists ONLY at the
// monorepo root (packages/mobile-shared has no node_modules at all), and the
// root's React (19.2.5) is a different physical copy than this app's local
// React (19.2.3). At runtime metro resolves zustand's `require('react')` to
// this app's copy first, so there's no bug in the running app — but jest's
// resolver walks zustand's require up to the hoisted root copy, producing
// two live Reacts in the same render tree and breaking hooks
// ("Cannot read properties of null (reading 'useCallback')") in any
// zustand-backed component. Pin `react` the same way as above.
// Same class of bug a fourth time, but for `expo-haptics`: Task 7 wired
// `@repo/mobile-shared/haptics/feedback` (adminHaptics) into SegmentedControl,
// which the `@/components/ui` barrel re-exports, so any test that touches the
// barrel now transitively requires expo-haptics from
// packages/mobile-shared/haptics/feedback.ts — which, having no local
// node_modules, resolves expo-haptics to the hoisted ROOT copy instead of
// this app's local install. That root copy's `expo` -> `expo-asset` chain
// calls a resolveAssetSource API this app's pinned react-native doesn't
// expose the same way, throwing "setCustomSourceTransformer is not a
// function" at require time. Pin `expo-haptics` the same way as above.
module.exports = {
  preset: 'jest-expo',
  moduleNameMapper: {
    '^react-native$': '<rootDir>/node_modules/react-native',
    '^react-native/(.*)$': '<rootDir>/node_modules/react-native/$1',
    '^@react-native-firebase/auth$': '<rootDir>/node_modules/@react-native-firebase/auth',
    '^@react-native-firebase/auth/(.*)$': '<rootDir>/node_modules/@react-native-firebase/auth/$1',
    '^@react-native-firebase/app$': '<rootDir>/node_modules/@react-native-firebase/app',
    '^@react-native-firebase/app/(.*)$': '<rootDir>/node_modules/@react-native-firebase/app/$1',
    '^react$': '<rootDir>/node_modules/react',
    '^react/(.*)$': '<rootDir>/node_modules/react/$1',
    '^expo-haptics$': '<rootDir>/node_modules/expo-haptics',
    '^expo-haptics/(.*)$': '<rootDir>/node_modules/expo-haptics/$1',
  },
};

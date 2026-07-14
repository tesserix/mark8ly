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
module.exports = {
  preset: 'jest-expo',
  moduleNameMapper: {
    '^react-native$': '<rootDir>/node_modules/react-native',
    '^react-native/(.*)$': '<rootDir>/node_modules/react-native/$1',
    '^@react-native-firebase/auth$': '<rootDir>/node_modules/@react-native-firebase/auth',
    '^@react-native-firebase/auth/(.*)$': '<rootDir>/node_modules/@react-native-firebase/auth/$1',
    '^@react-native-firebase/app$': '<rootDir>/node_modules/@react-native-firebase/app',
    '^@react-native-firebase/app/(.*)$': '<rootDir>/node_modules/@react-native-firebase/app/$1',
  },
};

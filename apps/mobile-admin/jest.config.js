// Pin `react-native` resolution to this app's local install. The monorepo
// root hoists a different react-native version (used by another app in the
// npm workspace), and jest-expo's transitive @react-native/jest-preset
// resolves `react-native` relative to *its own* location — which lands on
// the hoisted root copy instead of this app's version, breaking native
// module mocks (e.g. "PlatformConstants could not be found"). Overriding
// moduleNameMapper here forces every `react-native` (and subpath) import
// back to apps/mobile-admin/node_modules/react-native.
module.exports = {
  preset: 'jest-expo',
  moduleNameMapper: {
    '^react-native$': '<rootDir>/node_modules/react-native',
    '^react-native/(.*)$': '<rootDir>/node_modules/react-native/$1',
  },
};

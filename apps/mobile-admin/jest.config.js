const path = require('node:path');
const expoPreset = require('jest-expo/jest-preset');
const { createRequire } = require('node:module');

// Resolve SDK dependencies from this workspace after npm hoisting.
const packageRoot = (name) => path.dirname(require.resolve(`${name}/package.json`));
const expoRequire = createRequire(require.resolve('expo/package.json'));
const expoDependencyRoot = (name) => path.dirname(expoRequire.resolve(`${name}/package.json`));

module.exports = {
  preset: 'jest-expo',
  // Transform the ESM decoder behind the legacy CommonJS navigation bridge.
  transformIgnorePatterns: expoPreset.transformIgnorePatterns.map((pattern) =>
    pattern.replace('(?!(.pnpm|', '(?!(safe-decode-uri-component|.pnpm|'),
  ),
  moduleNameMapper: {
    '^react-native$': packageRoot('react-native'),
    '^react-native/(.*)$': `${packageRoot('react-native')}/$1`,
    '^@react-native-firebase/app$': packageRoot('@react-native-firebase/app'),
    '^@react-native-firebase/app/(.*)$': `${packageRoot('@react-native-firebase/app')}/$1`,
    '^react$': packageRoot('react'),
    '^react/(.*)$': `${packageRoot('react')}/$1`,
    '^expo-haptics$': packageRoot('expo-haptics'),
    '^expo-haptics/(.*)$': `${packageRoot('expo-haptics')}/$1`,
    '^expo-secure-store$': packageRoot('expo-secure-store'),
    '^expo-secure-store/(.*)$': `${packageRoot('expo-secure-store')}/$1`,
    '^expo/virtual/(.*)$': `${packageRoot('expo')}/virtual/$1`,
    '^expo-modules-core$': expoDependencyRoot('expo-modules-core'),
    '^expo-modules-core/(.*)$': `${expoDependencyRoot('expo-modules-core')}/$1`,
    '^expo-asset$': expoDependencyRoot('expo-asset'),
    '^expo-asset/(.*)$': `${expoDependencyRoot('expo-asset')}/$1`,
    '^expo$': packageRoot('expo'),
    '^expo/(.*)$': `${packageRoot('expo')}/$1`,
    '^@gorhom/bottom-sheet$': '<rootDir>/lib/test-support/gorhom-bottom-sheet-mock',
  },
};

// getSentryExpoConfig is a thin wrapper around expo's getDefaultConfig: it
// adds a Debug ID to the bundle so the source maps uploaded at build time can
// be matched to the JS that actually crashed. Without it a stack trace
// arrives as minified bundle offsets, which is no more useful than the native
// frames Apple already gives us — i.e. the whole reason for the SDK is lost.
//
// Swapped by hand rather than by `npx @sentry/wizard`. The wizard rewrites
// this file, and everything below is load-bearing: the NativeWind symlink
// resolution, package-exports staying off (enabling it crashed
// setUpDefaultReactNativeEnvironment at launch), hierarchical lookup staying
// on (disabling it broke nested resolution outright), and the @expo/ui
// mapping. Each was arrived at by debugging a specific failure.
const { getSentryExpoConfig } = require('@sentry/react-native/metro');

// NativeWind's tailwindcss@3 requirement is satisfied by a symlink created in
// the `postinstall` (scripts/link-nativewind-tailwind.js): npm hoists
// `nativewind` to the monorepo root where the web apps' tailwindcss@4 lives, so
// the postinstall points nativewind at this app's nested v3 copy. A previous
// approach patched `Module._resolveFilename` here, but that global override
// deadlocked Metro's graph build — the symlink keeps resolution untouched.

const { withNativeWind } = require('nativewind/metro');
const { FileStore } = require('metro-cache');
const path = require('path');

const projectRoot = __dirname;
const monorepoRoot = path.resolve(projectRoot, '../..');

const config = getSentryExpoConfig(projectRoot);

config.watchFolders = [monorepoRoot];
config.resolver.nodeModulesPaths = [
  path.resolve(projectRoot, 'node_modules'),
  path.resolve(monorepoRoot, 'node_modules'),
];
config.resolver.unstable_enableSymlinks = true;
// Keep package-exports resolution OFF (Metro default, matches Home-Chef).
// Enabling it made some deps resolve to an ESM entry Hermes evaluated to
// `undefined`, crashing setUpDefaultReactNativeEnvironment at launch.
// @repo/mobile-shared subpaths still resolve via classic path resolution.
config.resolver.unstable_enablePackageExports = false;
// Hierarchical lookup stays ON. It was previously disabled on the theory that
// crawling the monorepo-root node_modules made Metro hang. That diagnosis was
// wrong: the hang came from NativeWind's Tailwind CLI child process dying on a
// v4 install and never being reported (its `fork` handles only `message`, not
// `error`/`exit`), which is now prevented by the postinstall guard. Disabling it
// actively broke resolution of any NESTED dependency — Metro then sees only the
// two `nodeModulesPaths` roots — e.g. `color` requires color-string@^1.9.1,
// which npm nests because the root has 2.x, and bundling failed outright.
// With it enabled the bundle resolves and is FASTER (13.7s vs a 26.3s failure).
config.resolver.disableHierarchicalLookup = false;

// expo-router@56's Android/iOS native Stack toolbar imports
// `@expo/ui/{jetpack-compose,swift-ui}`. npm nests `@expo/ui` under
// `expo-router/node_modules`, which the app+root-only `nodeModulesPaths` above
// (with disableHierarchicalLookup) cannot reach, so Metro fails to resolve it
// (Android bundling hit it first; iOS slipped past). Map `@expo/ui` to wherever
// it actually installed, resolved relative to expo-router so it's robust to the
// hoist layout. `@expo/ui` ships physical `jetpack-compose/` + `swift-ui/` dirs,
// so this resolves without needing package-exports (kept off above).
const expoRouterDir = path.dirname(
  require.resolve('expo-router/package.json', { paths: [projectRoot] }),
);
config.resolver.extraNodeModules = {
  ...(config.resolver.extraNodeModules || {}),
  '@expo/ui': path.dirname(
    require.resolve('@expo/ui/package.json', { paths: [expoRouterDir] }),
  ),
};

config.cacheStores = [
  new FileStore({ root: path.join(projectRoot, '.metro-cache') }),
];

module.exports = withNativeWind(config, {
  input: './global.css',
});

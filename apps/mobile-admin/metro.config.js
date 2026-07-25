const { getDefaultConfig } = require('expo/metro-config');

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

const config = getDefaultConfig(projectRoot);

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
// Do NOT crawl parent node_modules hierarchically — nodeModulesPaths above
// already lists the two real roots. Without this, metro walks the entire
// monorepo-root node_modules and hangs on this large workspace.
config.resolver.disableHierarchicalLookup = true;

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

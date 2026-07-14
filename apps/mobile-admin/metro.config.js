const { getDefaultConfig } = require('expo/metro-config');

// npm hoists `nativewind` to the monorepo root, where `tailwindcss@4`
// (required by the web apps: admin/onboarding/storefront) lives. NativeWind
// only supports Tailwind v3. Redirect every `tailwindcss` resolution to this
// app's nested v3 copy for the Metro config process, so nativewind's
// version check and CSS compilation see v3 without disturbing the web apps.
const Module = require('module');
const appTailwindDir = require('path').dirname(
  require.resolve('tailwindcss/package.json', { paths: [__dirname] }),
);
const _origResolve = Module._resolveFilename;
Module._resolveFilename = function (request, ...args) {
  if (request === 'tailwindcss' || request.startsWith('tailwindcss/')) {
    return _origResolve.call(this, appTailwindDir + request.slice('tailwindcss'.length), ...args);
  }
  return _origResolve.call(this, request, ...args);
};

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
// @repo/mobile-shared ships subpath exports (api/client, auth/provider, …),
// so package-exports resolution must stay on.
config.resolver.unstable_enablePackageExports = true;
// Do NOT crawl parent node_modules hierarchically — nodeModulesPaths above
// already lists the two real roots. Without this, metro walks the entire
// monorepo-root node_modules and hangs on this large workspace.
config.resolver.disableHierarchicalLookup = true;

config.cacheStores = [
  new FileStore({ root: path.join(projectRoot, '.metro-cache') }),
];

module.exports = withNativeWind(config, {
  input: './global.css',
});

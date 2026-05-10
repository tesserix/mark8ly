const { getDefaultConfig } = require("expo/metro-config");
const path = require("path");

const projectRoot = __dirname;
const workspaceRoot = path.resolve(projectRoot, "../..");

const config = getDefaultConfig(projectRoot);

config.watchFolders = [workspaceRoot];
config.resolver.nodeModulesPaths = [
  path.resolve(projectRoot, "node_modules"),
  path.resolve(workspaceRoot, "node_modules"),
];

config.resolver.unstable_enablePackageExports = true;

// Hard-pin React, React-DOM, and react-native to mobile-admin's nested
// copies. The workspace root holds React 19 + RN-newer (used by web admin
// and other workspaces); Expo SDK 52 + RN 0.76 here require React 18.3.1.
// extraNodeModules only acts as a fallback, so we use resolveRequest to
// force every import to the local copy regardless of where the importer
// lives — otherwise packages walking up to the root pick React 19 and the
// app crashes with "Cannot read property 'ReactCurrentOwner' of undefined".
const FORCE = {
  react: path.resolve(projectRoot, "node_modules/react"),
  "react-dom": path.resolve(projectRoot, "node_modules/react-dom"),
};

const defaultResolveRequest = config.resolver.resolveRequest;
config.resolver.resolveRequest = (context, moduleName, platform) => {
  for (const [name, target] of Object.entries(FORCE)) {
    if (moduleName === name || moduleName.startsWith(`${name}/`)) {
      const sub = moduleName.slice(name.length);
      const filePath = sub ? path.join(target, sub) : target;
      return context.resolveRequest(context, filePath, platform);
    }
  }
  if (defaultResolveRequest) {
    return defaultResolveRequest(context, moduleName, platform);
  }
  return context.resolveRequest(context, moduleName, platform);
};

module.exports = config;

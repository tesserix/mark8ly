/* eslint-disable no-console */
// npm hoists `nativewind` (and its `react-native-css-interop` engine) to the
// monorepo root, where the web apps' `tailwindcss@4` lives. NativeWind only
// supports Tailwind v3. Rather than a global `Module._resolveFilename` shim
// (which deadlocks Metro's graph build), point each consumer at this app's
// nested `tailwindcss@3` by symlinking it into the consumer's own
// `node_modules`, so `require("tailwindcss")` resolves v3 without touching the
// web apps' v4. Idempotent: skips a consumer that already resolves v3.
const fs = require('fs');
const path = require('path');

const APP_ROOT = path.resolve(__dirname, '..');
const CONSUMERS = ['nativewind', 'react-native-css-interop'];

function resolveDir(pkg, fromPaths) {
  try {
    return path.dirname(
      require.resolve(`${pkg}/package.json`, { paths: fromPaths }),
    );
  } catch {
    return null;
  }
}

function versionAt(dir) {
  try {
    return require(path.join(dir, 'package.json')).version;
  } catch {
    return null;
  }
}

function main() {
  const appTailwind = resolveDir('tailwindcss', [APP_ROOT]);
  if (!appTailwind) {
    console.warn('[link-nativewind-tailwind] app-local tailwindcss not found; skipping');
    return;
  }
  const appTwVersion = versionAt(appTailwind);
  if (!appTwVersion || !appTwVersion.startsWith('3.')) {
    console.warn(
      `[link-nativewind-tailwind] app tailwindcss is ${appTwVersion}, expected 3.x; skipping`,
    );
    return;
  }

  for (const pkg of CONSUMERS) {
    const consumerDir = resolveDir(pkg, [APP_ROOT]);
    if (!consumerDir) continue; // consumer not installed — nothing to fix

    // Already resolves a v3 tailwindcss on its own path? Leave it.
    const seen = resolveDir('tailwindcss', [consumerDir]);
    if (seen && (versionAt(seen) || '').startsWith('3.')) continue;

    const linkDir = path.join(consumerDir, 'node_modules');
    const linkPath = path.join(linkDir, 'tailwindcss');
    fs.mkdirSync(linkDir, { recursive: true });
    try {
      fs.rmSync(linkPath, { recursive: true, force: true });
    } catch {
      /* nothing to remove */
    }
    const rel = path.relative(linkDir, appTailwind);
    fs.symlinkSync(rel, linkPath, 'dir');
    console.log(
      `[link-nativewind-tailwind] linked ${pkg} tailwindcss -> ${rel} (v${appTwVersion})`,
    );
  }
}

main();

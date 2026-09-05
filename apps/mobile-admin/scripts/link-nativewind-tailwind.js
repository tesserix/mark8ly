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
    // Fail, don't skip. NativeWind is Tailwind-v3-only, and its Metro
    // integration forks a child process for the Tailwind CLI and resolves only
    // on `message` — with no `error`/`exit` handler. So a child that dies on a
    // v4 install does not surface an error: Metro hangs forever at 0% CPU with
    // no output, in both `expo start` and the Xcode bundle phase. That is what
    // dependency bump #251 (tailwindcss 3 -> 4) caused, and this script warned
    // and returned 0, so nothing stopped it. An exit code here is the only
    // cheap signal between a bad bump and a silent, undebuggable hang.
    console.error(
      `[link-nativewind-tailwind] app-local tailwindcss is ${appTwVersion}, but ` +
        'NativeWind requires 3.x.\n' +
        '  Leaving this unresolved makes Metro hang indefinitely with no error.\n' +
        `  Pin "tailwindcss" to ^3.4.19 in ${path.join(APP_ROOT, 'package.json')}.`,
    );
    process.exitCode = 1;
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

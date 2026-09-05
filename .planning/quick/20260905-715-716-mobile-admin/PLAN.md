# mobile-admin: fix the launch crash (#715) and the build chain (#716)

`apps/mobile-admin` renders a blank screen on launch and cannot be built from a
clean checkout. Both were found while verifying #686; neither is migration work.

## Tasks

### 1. Align the Firebase packages and restore Tailwind v3
`apps/mobile-admin/package.json`. `@react-native-firebase/app` 26.2.0 against
`auth` 26.3.2 conflicts natively (`Firebase/Auth` 12.17.0 vs 12.18.0) and
CocoaPods warns it may crash at runtime. `tailwindcss` was bumped 3.4.19 -> 4.3.3
by #251 but NativeWind is v3-only and `global.css` is v3 syntax.
**Done when:** both firebase packages are on one version, `tailwindcss` is `^3.4.19`,
and `npm ci` reproduces it.

### 2. Make the NativeWind guard fail loudly
`scripts/link-nativewind-tailwind.js` skips with a warning when app-local
tailwind is not 3.x — which is how #251 landed silently. NativeWind's own
`getCSSForPlatform` forks a child and resolves only on `message`, with no
`error`/`exit` handler, so a child that dies takes Metro down into an infinite
0%-CPU hang with no output. The guard is the only thing standing between a
version bump and that hang.
**Done when:** a non-3.x app-local tailwind exits non-zero with a message naming
the consequence.

### 3. Migrate mobile-shared auth off the removed default export (#715)
`@react-native-firebase/auth` v26 has no default export; `gip.ts`,
`link.ts` and `social-credentials.ts` all do `import auth from ...` and call
`auth()` -> `undefined is not a function` inside `AuthProvider`'s `useState`
initialiser, before any screen mounts. Move to the modular API (`getAuth()` etc.)
and update the three test files that mock the default.
**Done when:** the app launches to the login screen on a simulator, and
`npm test` passes.

### 4. Unbreak `expo prebuild`
`@expo/plist` needs `@xmldom/xmldom@^0.8.x` and calls `parseFromString(xml)` with
one argument; npm hoists 0.9.10, where the mimeType argument is required, so
prebuild dies in `withIosInfoPlistBaseMod`.
Then re-run a CLEAN prebuild and re-check two things that may have been
side-effects of that failure rather than defects in their own right:
`ios.useFrameworks` not reaching `Podfile.properties.json`, and whether
`$RNFirebaseDisableSPM` is still needed once the Firebase versions agree.
Only add a config plugin for whatever genuinely remains.
**Done when:** `rm -rf ios && expo prebuild -p ios && pod install` succeeds from
a clean tree with no manual edits.

### 5. Verify end to end
`npm ci` -> clean prebuild -> pod install -> Release build -> launch -> sign in
as a real merchant against production. A green `tsc` and `jest` do NOT count:
both pass today while the app is broken (the module is mocked in tests and the
stale `.d.ts` still declares the default export).
**Done when:** an actual sign-in succeeds on the simulator.

### 6. Propose a gate
Three defects (#715, #251, the firebase mismatch) all arrived via automated
dependency bumps into an app nothing in CI builds or runs. Note the options; do
not add CI minutes without the owner's call.

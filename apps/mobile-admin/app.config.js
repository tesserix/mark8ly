const fs = require('fs');
const path = require('path');

// Demo-auth builds (EXPO_PUBLIC_AUTH_BACKEND=demo) skip the
// @react-native-firebase/app config plugin: its prebuild step requires a
// GoogleService-Info.plist we don't ship yet, and the demo auth backend never
// touches Firebase. Real GIP builds keep the plugin and provide the plist.
const USE_DEMO_AUTH = process.env.EXPO_PUBLIC_AUTH_BACKEND === 'demo';

// The Google Sign-In config plugin needs the iOS URL scheme at CONFIG time
// (during prebuild), before any EXPO_PUBLIC_* runtime env exists. Deriving it
// from the committed GoogleService-Info.plist (REVERSED_CLIENT_ID) keeps
// `expo config` / EAS cloud builds working WITHOUT relying on a gitignored
// .env.local — which the cloud never has. Falls back to the env override.
function iosUrlSchemeFromPlist() {
  try {
    const plist = fs.readFileSync(
      path.join(__dirname, 'GoogleService-Info.plist'),
      'utf8',
    );
    const m = plist.match(
      /<key>REVERSED_CLIENT_ID<\/key>\s*<string>([^<]+)<\/string>/,
    );
    return m ? m[1] : '';
  } catch {
    return '';
  }
}
const IOS_GOOGLE_URL_SCHEME =
  process.env.EXPO_PUBLIC_GOOGLE_IOS_URL_SCHEME || iosUrlSchemeFromPlist();

const PRODUCTION = {
  name: 'Mark8ly Admin',
  bundleIdentifier: 'com.mark8ly.admin',
  androidPackage: 'com.mark8ly.admin',
  extra: {
    apiBaseUrl: process.env.EXPO_PUBLIC_API_URL || 'https://api.mark8ly.com',
    storefrontUrl: 'https://mark8ly.com',
    adminWebUrl: 'https://admin.mark8ly.com',
    signupUrl: 'https://mark8ly.com',
    gipTenantId: process.env.GIP_TENANT_ID || '',
  },
};

module.exports = {
  expo: {
    name: PRODUCTION.name,
    // Expo account that owns the EAS project — same org as the Home-Chef
    // apps, so `eas login` / credentials are shared. `eas init` creates the
    // mark8ly-admin project under this org and fills extra.eas.projectId.
    owner: 'tesserix-org',
    slug: 'mark8ly-admin',
    scheme: 'mark8ly-admin',
    version: '1.0.0',
    orientation: 'portrait',
    icon: './assets/icon.png',
    userInterfaceStyle: 'light',
    newArchEnabled: true,
    jsEngine: 'hermes',
    splash: {
      image: './assets/splash.png',
      resizeMode: 'contain',
      backgroundColor: '#F7F6F2',
    },
    ios: {
      supportsTablet: false,
      bundleIdentifier: PRODUCTION.bundleIdentifier,
      infoPlist: {
        NSFaceIDUsageDescription: 'Use Face ID to unlock the admin app',
        NSCameraUsageDescription: 'Take product photos for your store',
        NSPhotoLibraryUsageDescription:
          'Select product images from your library',
        ITSAppUsesNonExemptEncryption: false,
      },
      associatedDomains: ['applinks:admin.mark8ly.com'],
      usesAppleSignIn: true,
      ...(USE_DEMO_AUTH
        ? {}
        : {
            googleServicesFile:
              process.env.GOOGLE_SERVICES_PLIST || './GoogleService-Info.plist',
          }),
    },
    android: {
      adaptiveIcon: {
        foregroundImage: './assets/adaptive-icon.png',
        backgroundColor: '#F7F6F2',
      },
      package: PRODUCTION.androidPackage,
      intentFilters: [
        {
          action: 'VIEW',
          autoVerify: true,
          data: [
            { scheme: 'https', host: 'admin.mark8ly.com', pathPrefix: '/' },
          ],
          category: ['BROWSABLE', 'DEFAULT'],
        },
      ],
    },
    plugins: [
      'expo-router',
      'expo-font',
      'expo-secure-store',
      'expo-local-authentication',
      'expo-image-picker',
      'expo-notifications',
      ['expo-build-properties', { ios: { newArchEnabled: true, useFrameworks: 'static' } }],
      ...(USE_DEMO_AUTH
        ? []
        : [
            '@react-native-firebase/app',
            'expo-apple-authentication',
            [
              '@react-native-google-signin/google-signin',
              { iosUrlScheme: IOS_GOOGLE_URL_SCHEME },
            ],
          ]),
    ],
    extra: {
      // extra.eas.projectId is intentionally absent until `eas init` creates
      // the project under tesserix-org and prints the real UUID to paste here.
      ...PRODUCTION.extra,
    },
  },
};

const PRODUCTION = {
  name: "Mark8ly Admin",
  bundleIdentifier: "com.mark8ly.admin",
  androidPackage: "com.mark8ly.admin",
  extra: {
    apiBaseUrl: "https://api.mark8ly.com",
    storefrontUrl: "https://mark8ly.com",
    adminWebUrl: "https://admin.mark8ly.com",
    signupUrl: "https://mark8ly.com",
    gipTenantId: process.env.GIP_TENANT_ID || "",
  },
};

module.exports = {
  expo: {
    name: PRODUCTION.name,
    slug: "mark8ly-admin",
    version: "1.0.0",
    orientation: "portrait",
    icon: "./assets/icon.png",
    scheme: "mark8ly-admin",
    userInterfaceStyle: "light",
    splash: {
      image: "./assets/splash.png",
      resizeMode: "contain",
      backgroundColor: "#F7F6F2",
    },
    ios: {
      supportsTablet: false,
      bundleIdentifier: PRODUCTION.bundleIdentifier,
      infoPlist: {
        NSCameraUsageDescription: "Take product photos for your store",
        NSPhotoLibraryUsageDescription: "Select product images from your library",
      },
      associatedDomains: ["applinks:admin.mark8ly.com"],
    },
    android: {
      adaptiveIcon: {
        foregroundImage: "./assets/adaptive-icon.png",
        backgroundColor: "#F7F6F2",
      },
      package: PRODUCTION.androidPackage,
      intentFilters: [
        {
          action: "VIEW",
          autoVerify: true,
          data: [{ scheme: "https", host: "admin.mark8ly.com", pathPrefix: "/" }],
          category: ["BROWSABLE", "DEFAULT"],
        },
      ],
    },
    plugins: [
      "expo-router",
      "expo-camera",
      "expo-image-picker",
      "expo-secure-store",
      "expo-notifications",
      "@react-native-firebase/app",
    ],
    extra: {
      eas: { projectId: "your-eas-project-id" },
      ...PRODUCTION.extra,
    },
  },
};

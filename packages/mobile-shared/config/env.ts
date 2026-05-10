import Constants from "expo-constants";

export interface EnvConfig {
  apiBaseUrl: string;
  storefrontUrl: string;
  adminWebUrl: string;
  signupUrl: string;
  gipTenantId: string;
}

const PRODUCTION: EnvConfig = {
  apiBaseUrl: "https://api.mark8ly.com",
  storefrontUrl: "https://mark8ly.com",
  adminWebUrl: "https://admin.mark8ly.com",
  signupUrl: "https://mark8ly.com",
  gipTenantId: "",
};

interface ExpoExtra {
  apiBaseUrl?: string;
  storefrontUrl?: string;
  adminWebUrl?: string;
  signupUrl?: string;
  gipTenantId?: string;
}

function readBuildTimeEnv(): EnvConfig {
  const extra = (Constants.expoConfig?.extra ?? {}) as ExpoExtra;
  return {
    apiBaseUrl: extra.apiBaseUrl ?? PRODUCTION.apiBaseUrl,
    storefrontUrl: extra.storefrontUrl ?? PRODUCTION.storefrontUrl,
    adminWebUrl: extra.adminWebUrl ?? PRODUCTION.adminWebUrl,
    signupUrl: extra.signupUrl ?? PRODUCTION.signupUrl,
    gipTenantId: extra.gipTenantId ?? PRODUCTION.gipTenantId,
  };
}

let cached: EnvConfig | null = null;

export function getEnv(): EnvConfig {
  if (!cached) cached = readBuildTimeEnv();
  return cached;
}

export function useEnvironment(): EnvConfig {
  return getEnv();
}

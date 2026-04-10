import * as SecureStore from "expo-secure-store";
import type { AuthUser } from "../stores/auth-store";

const KEYS = {
  TOKEN: "mark8ly_auth_token",
  USER: "mark8ly_auth_user",
} as const;

export async function saveToken(token: string): Promise<void> {
  await SecureStore.setItemAsync(KEYS.TOKEN, token);
}

export async function getToken(): Promise<string | null> {
  return SecureStore.getItemAsync(KEYS.TOKEN);
}

export async function deleteToken(): Promise<void> {
  await SecureStore.deleteItemAsync(KEYS.TOKEN);
}

export async function saveUser(user: AuthUser): Promise<void> {
  await SecureStore.setItemAsync(KEYS.USER, JSON.stringify(user));
}

export async function getSavedUser(): Promise<AuthUser | null> {
  const raw = await SecureStore.getItemAsync(KEYS.USER);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as AuthUser;
  } catch {
    return null;
  }
}

export async function deleteSavedUser(): Promise<void> {
  await SecureStore.deleteItemAsync(KEYS.USER);
}

export async function clearAuthStorage(): Promise<void> {
  await Promise.all([deleteToken(), deleteSavedUser()]);
}

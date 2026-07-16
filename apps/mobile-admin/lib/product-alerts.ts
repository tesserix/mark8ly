import { Alert } from "react-native";
import { ApiError } from "@repo/mobile-shared/api/client";

/**
 * This branch went to real trouble to make contract-mismatch messages name
 * the offending field (see client.ts's ApiError construction) — surface that
 * instead of a generic string wherever we have it.
 */
export function getErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.message;
  return fallback;
}

/**
 * react-query mutate() options that surface a failure as an Alert. A silent
 * mutation failure — the UI reverting on refetch with no word to the merchant —
 * is the exact bug class this branch exists to kill, and these routes have real
 * reachable failures (a 400 on removing an option's last value, a 429 from the
 * 60 req/min limiter, network/500).
 */
export function alertOnError(fallback: string) {
  return {
    onError: (err: unknown) => {
      Alert.alert("Error", getErrorMessage(err, fallback));
    },
  };
}

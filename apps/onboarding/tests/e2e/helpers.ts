import { expect, type APIRequestContext } from "@playwright/test";

/**
 * Helpers for the onboarding e2e suite.
 *
 * The platform-api exposes a non-prod-only test endpoint at
 * /api/v1/test/verification/latest?email=... that returns the most-recent
 * plaintext magic-link token for an email. We use it to bypass the inbox
 * during e2e runs.
 */

export const API_URL =
  process.env.API_URL ?? "http://localhost:8086";

/** A unique email per test run so Postgres state from a previous run
 * doesn't collide with the new one. The slug is derived from the local
 * part so it stays under 63 chars and matches the slug regex.
 *
 * Phase M added a required password field on the signup form, so each
 * generated identity also carries a default e2e password. */
export function uniqueEmail(label = "e2e"): {
  email: string;
  slug: string;
  businessName: string;
  password: string;
} {
  const stamp = `${Date.now().toString(36)}${Math.floor(
    Math.random() * 1e4,
  ).toString(36)}`;
  const local = `${label}-${stamp}`;
  return {
    email: `${local}@example.com`,
    slug: local.replace(/[^a-z0-9-]/g, "").slice(0, 60),
    businessName: `${label} ${stamp}`,
    password: "e2e-test-password-123",
  };
}

/**
 * Poll the test helper endpoint until a magic-link token shows up for
 * the given email. The token is created synchronously inside the
 * onboarding submit handler, so this almost always returns on the first
 * attempt — but we retry briefly to absorb cold-start jitter.
 */
export async function fetchMagicLinkToken(
  request: APIRequestContext,
  email: string,
  { timeoutMs = 5000 }: { timeoutMs?: number } = {},
): Promise<string> {
  const deadline = Date.now() + timeoutMs;
  let lastStatus = 0;
  while (Date.now() < deadline) {
    const res = await request.get(
      `${API_URL}/api/v1/test/verification/latest?email=${encodeURIComponent(email)}`,
    );
    lastStatus = res.status();
    if (res.ok()) {
      const body = (await res.json()) as { data: { token: string } };
      expect(body.data.token, "test helper returned empty token").toBeTruthy();
      return body.data.token;
    }
    if (res.status() !== 404) {
      throw new Error(`test helper failed: ${res.status()}`);
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(
    `magic-link token never appeared for ${email} (last status ${lastStatus})`,
  );
}

import { describe, expect, it } from "vitest";
import { encodeSession } from "@/lib/session";
import {
  JOIN_GRANT_TTL_SECONDS,
  signJoinGrant,
  verifyJoinGrant,
} from "@/lib/auth/join-grant";

const CLAIMS = {
  uid: "uid-1",
  email: "shopper@example.com",
  store_slug: "store-two",
  store_id: "store-2",
  tenant_id: "tenant-2",
};

describe("join grant", () => {
  it("round-trips the identity it was signed for", () => {
    expect(verifyJoinGrant(signJoinGrant(CLAIMS), "store-two")).toEqual(CLAIMS);
  });

  it("rejects a grant spent at a different store", () => {
    // A grant is minted at the store where the membership gate refused.
    // Without this check it would authorise a join anywhere.
    expect(verifyJoinGrant(signJoinGrant(CLAIMS), "store-three")).toBeNull();
  });

  it("rejects a tampered payload", () => {
    const grant = signJoinGrant(CLAIMS);
    const sig = grant.slice(grant.lastIndexOf(".") + 1);
    const forged = Buffer.from(
      JSON.stringify({
        ...CLAIMS,
        email: "victim@example.com",
        purpose: "customer-store-join-grant:v1",
        exp: Math.floor(Date.now() / 1000) + 600,
      }),
    ).toString("base64");
    expect(verifyJoinGrant(`${forged}.${sig}`, "store-two")).toBeNull();
  });

  it("rejects an expired grant", () => {
    expect(verifyJoinGrant(signJoinGrant(CLAIMS, -1), "store-two")).toBeNull();
  });

  it("expires in minutes, not the session's 30 days", () => {
    expect(JOIN_GRANT_TTL_SECONDS).toBeLessThanOrEqual(15 * 60);
  });

  it("refuses a session cookie replayed as a join grant", () => {
    // Both are signed with SESSION_ENCRYPT_KEY, so the signature alone
    // would verify; the purpose tag is what keeps the two apart.
    const session = encodeSession({
      uid: CLAIMS.uid,
      email: CLAIMS.email,
      store_slug: CLAIMS.store_slug,
      store_id: CLAIMS.store_id,
      tenant_id: CLAIMS.tenant_id,
    });
    expect(verifyJoinGrant(session, "store-two")).toBeNull();
  });

  it("rejects junk without throwing", () => {
    for (const junk of ["", "no-dot", "a.b", undefined, null]) {
      expect(verifyJoinGrant(junk as string, "store-two")).toBeNull();
    }
  });
});

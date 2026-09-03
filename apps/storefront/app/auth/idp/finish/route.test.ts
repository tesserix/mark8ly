import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/slug", () => ({
  resolveStoreSlug: vi.fn(),
}));

vi.mock("@/app/sign-in/actions", () => ({
  resolveStore: vi.fn(),
  completeCustomerSignIn: vi.fn(),
}));

vi.mock("@/lib/auth/auth-bff-customer", async () => {
  const actual = await vi.importActual<
    typeof import("@/lib/auth/auth-bff-customer")
  >("@/lib/auth/auth-bff-customer");
  return { ...actual, finishCustomerIDPIntent: vi.fn() };
});

import { resolveStoreSlug } from "@/lib/slug";
import { completeCustomerSignIn, resolveStore } from "@/app/sign-in/actions";
import { finishCustomerIDPIntent } from "@/lib/auth/auth-bff-customer";
import { GET } from "./route";

const resolveStoreSlugMock = vi.mocked(resolveStoreSlug);
const resolveStoreMock = vi.mocked(resolveStore);
const completeCustomerSignInMock = vi.mocked(completeCustomerSignIn);
const finishCustomerIDPIntentMock = vi.mocked(finishCustomerIDPIntent);

const HOST = "shop.mark8ly.com";
const STORE = { tenant_id: "tenant-1", store_id: "store-1" };

function makeRequest(search: string): NextRequest {
  return new NextRequest(`https://${HOST}/auth/idp/finish${search}`, {
    headers: { host: HOST, "x-forwarded-host": HOST, "x-forwarded-proto": "https" },
  });
}

beforeEach(() => {
  resolveStoreSlugMock.mockReset();
  resolveStoreMock.mockReset();
  completeCustomerSignInMock.mockReset();
  finishCustomerIDPIntentMock.mockReset();

  resolveStoreSlugMock.mockResolvedValue("shop");
  resolveStoreMock.mockResolvedValue(STORE);
  completeCustomerSignInMock.mockResolvedValue({ ok: true });
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("GET /auth/idp/finish — success", () => {
  it("mints the session via completeCustomerSignIn (the shared helper), then redirects to dest", async () => {
    finishCustomerIDPIntentMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: "customer@example.com",
    });

    const res = await GET(makeRequest("?id=intent-1&token=tok-1&dest=%2Faccount"));

    expect(completeCustomerSignInMock).toHaveBeenCalledTimes(1);
    expect(completeCustomerSignInMock).toHaveBeenCalledWith(
      STORE,
      HOST,
      "shop",
      { uid: "u1", email: "customer@example.com" },
    );
    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toBe(`https://${HOST}/account`);
  });

  it("defaults to /account when no dest is given", async () => {
    finishCustomerIDPIntentMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: "customer@example.com",
    });

    const res = await GET(makeRequest("?id=intent-1&token=tok-1"));

    expect(res.headers.get("location")).toBe(`https://${HOST}/account`);
  });

  it("falls back to /account for a dest outside the fixed allowlist (open-redirect guard)", async () => {
    finishCustomerIDPIntentMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: "customer@example.com",
    });

    const res = await GET(
      makeRequest("?id=intent-1&token=tok-1&dest=https%3A%2F%2Fevil.example.com"),
    );

    expect(res.headers.get("location")).toBe(`https://${HOST}/account`);
  });

  it("respects the /account/security dest (SecurityClient's link flow)", async () => {
    finishCustomerIDPIntentMock.mockResolvedValue({
      kind: "complete",
      uid: "u1",
      email: "customer@example.com",
    });

    const res = await GET(
      makeRequest("?id=intent-1&token=tok-1&dest=%2Faccount%2Fsecurity"),
    );

    expect(res.headers.get("location")).toBe(`https://${HOST}/account/security`);
  });
});

describe("GET /auth/idp/finish — a tampered `user` param changes nothing", () => {
  it("finishCustomerIDPIntent is called with only id/token — never a user field — regardless of what `user` says", async () => {
    finishCustomerIDPIntentMock.mockResolvedValue({
      kind: "complete",
      uid: "real-uid",
      email: "real@example.com",
    });

    const res = await GET(
      makeRequest(
        "?id=intent-1&token=tok-1&user=" + encodeURIComponent("attacker@evil.com"),
      ),
    );

    expect(finishCustomerIDPIntentMock).toHaveBeenCalledWith({
      intentId: "intent-1",
      intentToken: "tok-1",
    });
    expect(finishCustomerIDPIntentMock.mock.calls[0]![0]).not.toHaveProperty("user");
    // The session is minted from finishCustomerIDPIntent's resolved
    // identity, never from the tampered `user` query param.
    expect(completeCustomerSignInMock).toHaveBeenCalledWith(
      STORE,
      HOST,
      "shop",
      { uid: "real-uid", email: "real@example.com" },
    );
    expect(res.status).toBe(303);
  });
});

describe("GET /auth/idp/finish — failure sets no cookie", () => {
  it("a rejected outcome (e.g. email_taken) never calls completeCustomerSignIn and redirects with that code", async () => {
    finishCustomerIDPIntentMock.mockResolvedValue({
      kind: "failed",
      code: "email_taken",
    });

    const res = await GET(makeRequest("?id=intent-1&token=tok-1"));

    expect(completeCustomerSignInMock).not.toHaveBeenCalled();
    expect(res.status).toBe(303);
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("/sign-in");
    expect(location).toContain("error=email_taken");
  });

  it("email_not_verified is distinguishable from email_taken in the redirect", async () => {
    finishCustomerIDPIntentMock.mockResolvedValue({
      kind: "failed",
      code: "email_not_verified",
    });

    const res = await GET(makeRequest("?id=intent-1&token=tok-1"));

    expect(completeCustomerSignInMock).not.toHaveBeenCalled();
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("error=email_not_verified");
    expect(location).not.toContain("email_taken");
  });

  it("a thrown AuthBffCustomerError never calls completeCustomerSignIn and redirects with a generic code, not the internal detail", async () => {
    finishCustomerIDPIntentMock.mockRejectedValue(new Error("ECONNREFUSED"));

    const res = await GET(makeRequest("?id=intent-1&token=tok-1"));

    expect(completeCustomerSignInMock).not.toHaveBeenCalled();
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("/sign-in");
    expect(location).toContain("error=zitadel_unavailable");
    expect(location).not.toContain("ECONNREFUSED");
  });

  it("Zitadel's own failure redirect (error param, no token) never calls auth-bff or completeCustomerSignIn", async () => {
    const res = await GET(
      makeRequest(
        "?id=intent-1&error=access_denied&error_description=" +
          encodeURIComponent("user cancelled at Google — internal detail"),
      ),
    );

    expect(finishCustomerIDPIntentMock).not.toHaveBeenCalled();
    expect(completeCustomerSignInMock).not.toHaveBeenCalled();
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("/sign-in");
    // The raw error_description must never reach the redirect the
    // browser follows.
    expect(location).not.toContain("cancelled");
    expect(location).not.toContain("internal detail");
  });

  it("missing id/token redirects with invalid_request and calls nothing downstream", async () => {
    const res = await GET(makeRequest(""));

    expect(resolveStoreSlugMock).not.toHaveBeenCalled();
    expect(finishCustomerIDPIntentMock).not.toHaveBeenCalled();
    expect(completeCustomerSignInMock).not.toHaveBeenCalled();
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("error=invalid_request");
  });

  it("an unresolvable store redirects with store_not_found and never calls auth-bff", async () => {
    resolveStoreMock.mockResolvedValue(null);

    const res = await GET(makeRequest("?id=intent-1&token=tok-1"));

    expect(finishCustomerIDPIntentMock).not.toHaveBeenCalled();
    expect(completeCustomerSignInMock).not.toHaveBeenCalled();
    expect(res.headers.get("location")).toContain("error=store_not_found");
  });
});

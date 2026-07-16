import { mediaUploadUrlSchema } from "@repo/mobile-shared/api/schemas/products";

// Real shape of `POST /products/{id}/media/upload-url` (UploadURLResponse,
// validation.go:68-72). The field is `url`, not `upload_url` — that alias
// only exists on the web admin's Next proxy route.
const REAL_RESPONSE = {
  url: "https://storage.googleapis.com/mark8ly-media/tenants/x/upload.jpg?X-Goog-Signature=abc",
  storage_key: "tenants/8b69eea9/products/a28defe3/upload.jpg",
  expires_at: "2026-07-16T12:00:00Z",
};

describe("mediaUploadUrlSchema", () => {
  it("parses the real upload-url response shape", () => {
    const parsed = mediaUploadUrlSchema.parse(REAL_RESPONSE);
    expect(parsed.storage_key).toBe(REAL_RESPONSE.storage_key);
    expect(parsed.url).toBe(REAL_RESPONSE.url);
  });

  it("rejects a payload missing storage_key", () => {
    const { storage_key, ...rest } = REAL_RESPONSE;
    const result = mediaUploadUrlSchema.safeParse(rest);
    expect(result.success).toBe(false);
  });

  it("rejects a payload missing url", () => {
    const { url, ...rest } = REAL_RESPONSE;
    const result = mediaUploadUrlSchema.safeParse(rest);
    expect(result.success).toBe(false);
  });
});

// product-crud.ts also exports react-query hooks that pull in the real api
// client (and, transitively, Firebase auth) — irrelevant to this pure
// orchestration function, but the module-level import chain still needs to
// resolve under jest. Mock it the same way use-products.test.tsx does.
jest.mock("@/lib/api-client", () => ({
  useApiClient: () => ({}),
}));

import { uploadProductMedia } from "@/lib/admin-api/product-crud";

const SIGNED_URL_RESPONSE = {
  url: "https://storage.googleapis.com/mark8ly-media/tenants/x/photo.jpg?sig=abc",
  storage_key: "tenants/x/photo.jpg",
  expires_at: "2026-07-16T12:00:00Z",
};

const CREATED_MEDIA = {
  id: "media-1",
  url: "https://cdn.mark8ly.com/tenants/x/photo.jpg",
  storage_key: "tenants/x/photo.jpg",
  position: 0,
  media_type: "image",
};

function makeFakeApi() {
  return {
    createMediaUploadUrl: jest.fn().mockResolvedValue(SIGNED_URL_RESPONSE),
    createMedia: jest.fn().mockResolvedValue(CREATED_MEDIA),
  };
}

describe("uploadProductMedia", () => {
  it("calls the 3 steps in order: upload-url, PUT, finalize", async () => {
    const calls: string[] = [];
    const api = makeFakeApi();
    api.createMediaUploadUrl.mockImplementation(async () => {
      calls.push("upload-url");
      return SIGNED_URL_RESPONSE;
    });
    const uploadFn = jest.fn(async () => {
      calls.push("put");
      return { status: 200 };
    });
    api.createMedia.mockImplementation(async () => {
      calls.push("finalize");
      return CREATED_MEDIA;
    });

    await uploadProductMedia(
      api,
      { productId: "product-1", asset: { uri: "file:///tmp/photo.jpg" }, position: 0 },
      uploadFn,
    );

    expect(calls).toEqual(["upload-url", "put", "finalize"]);
  });

  // The single most important assertion in this file: step 3 must send the
  // raw storage_key, NEVER a CDN URL. The backend builds the public URL
  // itself and ignores this field (service_single_media.go:91-97) — a future
  // reader is most likely to "helpfully" put a CDN URL here.
  it("sends url: storage_key (NOT a CDN URL) to the finalize step", async () => {
    const api = makeFakeApi();
    const uploadFn = jest.fn().mockResolvedValue({ status: 200 });

    await uploadProductMedia(
      api,
      { productId: "product-1", asset: { uri: "file:///tmp/photo.jpg" }, position: 2 },
      uploadFn,
    );

    expect(api.createMedia).toHaveBeenCalledWith(
      "product-1",
      expect.objectContaining({
        storage_key: SIGNED_URL_RESPONSE.storage_key,
        url: SIGNED_URL_RESPONSE.storage_key,
        position: 2,
      }),
    );
    const call = api.createMedia.mock.calls[0];
    if (!call) throw new Error("expected createMedia to have been called");
    expect(call[1].url).not.toContain("cdn.mark8ly.com");
  });

  it("PUTs to the signed url from step 1, not the api base url", async () => {
    const api = makeFakeApi();
    const uploadFn = jest.fn().mockResolvedValue({ status: 200 });

    await uploadProductMedia(
      api,
      { productId: "product-1", asset: { uri: "file:///tmp/photo.jpg" }, position: 0 },
      uploadFn,
    );

    expect(uploadFn).toHaveBeenCalledWith(
      SIGNED_URL_RESPONSE.url,
      "file:///tmp/photo.jpg",
      expect.any(String),
    );
  });

  it("rejects and never finalizes when the PUT fails", async () => {
    const api = makeFakeApi();
    const uploadFn = jest.fn().mockResolvedValue({ status: 403 });

    await expect(
      uploadProductMedia(
        api,
        { productId: "product-1", asset: { uri: "file:///tmp/photo.jpg" }, position: 0 },
        uploadFn,
      ),
    ).rejects.toThrow();

    expect(api.createMedia).not.toHaveBeenCalled();
  });

  it("infers content_type from the asset mimeType", async () => {
    const api = makeFakeApi();
    const uploadFn = jest.fn().mockResolvedValue({ status: 200 });

    await uploadProductMedia(
      api,
      {
        productId: "product-1",
        asset: { uri: "file:///tmp/photo.png", mimeType: "image/png" },
        position: 0,
      },
      uploadFn,
    );

    expect(api.createMediaUploadUrl).toHaveBeenCalledWith(
      "product-1",
      expect.objectContaining({ content_type: "image/png" }),
    );
  });
});

// tsconfig scopes `types` to ["jest"] only, so Node's ambient globals aren't
// picked up automatically — declare the one this file's fs.readFileSync
// call below needs, rather than widening the project-wide tsconfig.
declare const __dirname: string;

describe("image picker options", () => {
  it("requests the system cropper and never asks for library permission", () => {
    // Guard for 51d2e80b: requestMediaLibraryPermissionsAsync opts into the
    // legacy flow, where "Limited Access" strands the user in iOS's
    // limited-library management sheet and the real picker never opens.
    //
    // Moved from app/(tabs)/products/[id].tsx to lib/hooks/use-product-media-handlers.ts
    // (Task 5's enabling extraction, to keep [id].tsx under its pinned
    // line-count gate) — repointed here, same guard, same behaviour.
    const source = require("fs").readFileSync(
      require("path").join(__dirname, "../lib/hooks/use-product-media-handlers.ts"),
      "utf8",
    );
    expect(source).toContain("allowsEditing: true");
    expect(source).not.toContain("requestMediaLibraryPermissionsAsync");
  });
});

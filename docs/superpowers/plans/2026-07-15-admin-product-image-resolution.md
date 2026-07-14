# Web-admin Product Image Resolution Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the admin product editor from destroying merchants' high-resolution originals — upload the pristine file on add, make cropping optional/non-square, and let cropped renditions keep their source format.

**Architecture:** The backend already preserves whatever the add flow finalizes as `gcs_path_original`, and the recrop endpoints already crop non-destructively. So the core fix is frontend: the add flow uploads the raw `File` instead of a forced 1:1 square, and cropping moves entirely to the existing per-card recrop action. One additive, backward-compatible backend change lets the recrop endpoint sign the PUT with the source content-type so PNG/WebP crops keep their format.

**Tech Stack:** Next.js 16 / React 19 admin app (`apps/admin`, Vitest + Playwright), Go 1.26 `services/marketplace-api` (Gin + GORM, testify-free stdlib tests, `//go:build integration` for DB-backed tests).

## Global Constraints

- **Immutability:** never mutate objects/arrays in place; return new copies (spread). Applies to React state and form field updates.
- **TypeScript:** explicit types on exported functions/props; no `any` (use `unknown` + narrow); `interface` for object shapes, `type` for unions; string-literal unions over enums.
- **No `console.log`** in production code.
- **File size:** keep files focused (<800 lines; these are all far under).
- **Commits:** single-line conventional messages (`feat:`/`fix:`/`test:`/`refactor:`), **no signatures, no `-S`/`--signoff`**. Commit directly to `main`. No PR.
- **Backend media formats allowlist:** `image/png`, `image/jpeg`, `image/webp` only (matches existing upload-url contract).
- **Min-resolution threshold:** `1000px` short edge. Guard is **advisory only** — warn, never block, never throw.
- **Crop rendition:** preserve source format; **JPEG quality `0.95`** fallback (up from 0.92).
- **Aspect options:** `4:5` (default), `1:1`, `3:4`, `Free` (no fixed aspect).

Run all frontend unit tests from `apps/admin`: `npx vitest run <path>`.
Run backend integration tests from `services/marketplace-api`: `go test -tags=integration ./internal/product/... -run <Name>` (needs a test DB; CI provides it).

---

### Task 1: Backend — recrop signs the caller's content-type

Make the recrop endpoint accept an optional `content_type` and sign the PUT URL with it (defaulting to `image/jpeg`), so a WebP/PNG cropped blob PUT succeeds against the V4-signed URL (GCS binds content-type into the signature).

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/validation.go` (`RecropMediaRequest`, ~line 106)
- Modify: `services/marketplace-api/internal/handlers/admin/media.go` (`Recrop`, ~line 229)
- Modify: `services/marketplace-api/internal/product/service_media_recrop.go` (`RecropMediaRequest` + `RecropMedia`)
- Test: `services/marketplace-api/internal/product/service_media_recrop_integration_test.go` (new)

**Interfaces:**
- Consumes: existing `media.SignedURLGenerator.SignedUploadURL(ctx, key, contentType, ttl)`, `media.SignedReadURLGenerator`, `product.NewService`, `testdb.NewTx`, `buildService`, `seedStore`, `seedProductForMedia` (integration test helpers).
- Produces: `product.RecropMediaRequest.ContentType string` field; wire `RecropMediaRequest.ContentType` JSON `content_type`. Default `"image/jpeg"` when empty.

- [ ] **Step 1: Write the failing integration test**

Create `services/marketplace-api/internal/product/service_media_recrop_integration_test.go`:

```go
//go:build integration

package product_test

import (
	"context"
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// capturingUploader is a FakeUploader that also implements the signer
// interfaces so RecropMedia runs its full path and records the
// content-type handed to the PUT signer.
type capturingUploader struct {
	*media.FakeUploader
	gotPutContentType string
}

func (c *capturingUploader) SignedUploadURL(_ context.Context, key, contentType string, _ time.Duration) (string, time.Time, error) {
	c.gotPutContentType = contentType
	return "https://put.example/" + key, time.Now().Add(time.Minute), nil
}

func (c *capturingUploader) SignedReadURL(_ context.Context, key string, _ time.Duration) (string, time.Time, error) {
	return "https://get.example/" + key, time.Now().Add(time.Minute), nil
}

func seedMediaRow(t *testing.T, svc *product.Service, agg *product.Aggregate, storeID, tenantID, key string) *product.Media {
	t.Helper()
	row, err := svc.AddMedia(context.Background(), product.AddMediaRequest{
		ProductID:  agg.Product.ID,
		StoreID:    storeID,
		TenantID:   tenantID,
		StorageKey: key,
		URL:        "https://cdn/" + key,
	})
	if err != nil {
		t.Fatalf("seed media: %v", err)
	}
	return row
}

func TestIntegration_RecropMedia_SignsWithGivenContentType(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	up := &capturingUploader{FakeUploader: media.NewFakeUploader()}
	up.Register(media.Attrs{StorageKey: "orig-webp", Size: 10, ContentType: "image/webp"})
	svc := buildService(tx, up)
	agg := seedProductForMedia(t, svc, storeID, tenantID)
	row := seedMediaRow(t, svc, agg, storeID, tenantID, "orig-webp")

	_, err := svc.RecropMedia(context.Background(), product.RecropMediaRequest{
		ProductID:   agg.Product.ID,
		MediaID:     row.ID,
		StoreID:     storeID,
		TenantID:    tenantID,
		ContentType: "image/webp",
	}, up, up, time.Minute)
	if err != nil {
		t.Fatalf("recrop: %v", err)
	}
	if up.gotPutContentType != "image/webp" {
		t.Fatalf("put content-type = %q, want image/webp", up.gotPutContentType)
	}
}

func TestIntegration_RecropMedia_DefaultsContentTypeToJPEG(t *testing.T) {
	tx := testdb.NewTx(t)
	storeID, tenantID := seedStore(t, tx)
	up := &capturingUploader{FakeUploader: media.NewFakeUploader()}
	up.Register(media.Attrs{StorageKey: "orig-jpg", Size: 10, ContentType: "image/jpeg"})
	svc := buildService(tx, up)
	agg := seedProductForMedia(t, svc, storeID, tenantID)
	row := seedMediaRow(t, svc, agg, storeID, tenantID, "orig-jpg")

	_, err := svc.RecropMedia(context.Background(), product.RecropMediaRequest{
		ProductID: agg.Product.ID,
		MediaID:   row.ID,
		StoreID:   storeID,
		TenantID:  tenantID,
		// ContentType intentionally empty.
	}, up, up, time.Minute)
	if err != nil {
		t.Fatalf("recrop: %v", err)
	}
	if up.gotPutContentType != "image/jpeg" {
		t.Fatalf("put content-type = %q, want image/jpeg default", up.gotPutContentType)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/marketplace-api && go test -tags=integration ./internal/product/... -run TestIntegration_RecropMedia`
Expected: FAIL — `product.RecropMediaRequest` has no field `ContentType` (compile error).

- [ ] **Step 3: Add `ContentType` to the service request and use it**

In `services/marketplace-api/internal/product/service_media_recrop.go`, add the field to `RecropMediaRequest`:

```go
type RecropMediaRequest struct {
	ProductID   string
	MediaID     string
	StoreID     string
	TenantID    string
	Filename    string
	ContentType string
}
```

Then in `RecropMedia`, replace the hardcoded `"image/jpeg"` PUT signing. After the `filename` block and before signing, add:

```go
	contentType := req.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}
```

Change the PUT signing call from:

```go
	putURL, expiresAt, err := putSigner.SignedUploadURL(ctx, newKey, "image/jpeg", ttl)
```

to:

```go
	putURL, expiresAt, err := putSigner.SignedUploadURL(ctx, newKey, contentType, ttl)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/marketplace-api && go test -tags=integration ./internal/product/... -run TestIntegration_RecropMedia`
Expected: PASS (both cases).

- [ ] **Step 5: Thread `content_type` through the wire + handler**

In `services/marketplace-api/internal/handlers/admin/validation.go`, add the field to the wire `RecropMediaRequest` (~line 106):

```go
type RecropMediaRequest struct {
	CropBox     CropBox `json:"crop_box" binding:"required"`
	Rotation    int     `json:"rotation"`
	Filename    string  `json:"filename" binding:"omitempty,max=200"`
	ContentType string  `json:"content_type" binding:"omitempty,oneof=image/png image/jpeg image/webp"`
}
```

In `services/marketplace-api/internal/handlers/admin/media.go`, `Recrop` (~line 229), pass it into the service request:

```go
	svcReq := product.RecropMediaRequest{
		ProductID:   productID,
		MediaID:     mediaID,
		StoreID:     storeID,
		TenantID:    tenantID,
		Filename:    req.Filename,
		ContentType: req.ContentType,
	}
```

- [ ] **Step 6: Verify the package still builds and vets**

Run: `cd services/marketplace-api && go build ./... && go vet ./internal/handlers/admin/... ./internal/product/...`
Expected: no output (success).

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/validation.go services/marketplace-api/internal/handlers/admin/media.go services/marketplace-api/internal/product/service_media_recrop.go services/marketplace-api/internal/product/service_media_recrop_integration_test.go
git commit -m "feat(marketplace-api): recrop signs PUT with caller content_type (default image/jpeg)"
```

---

### Task 2: Frontend — `cropToBlob` preserves source format at quality 0.95

Let the canvas crop emit the source MIME type at quality `0.95`, defaulting to JPEG `0.95`.

**Files:**
- Modify: `apps/admin/components/products/media/cropImage.ts`
- Test: `apps/admin/components/products/media/cropImage.test.ts`

**Interfaces:**
- Produces: `cropToBlob(image, box, rotationDeg, opts?: { mimeType?: string; quality?: number }): Promise<Blob>`. Defaults: `mimeType = "image/jpeg"`, `quality = 0.95`. Existing 3-arg calls keep working.

- [ ] **Step 1: Write the failing test**

Append to `apps/admin/components/products/media/cropImage.test.ts` (inside the `describe("cropToBlob", …)` block):

```ts
  it("forwards source mimeType and 0.95 quality to toBlob", async () => {
    const toBlob = vi.fn((cb: (b: Blob) => void, _type?: string, _q?: number) => {
      cb(new Blob(["x"], { type: "image/webp" }));
    });
    const fakeCtx = { drawImage: vi.fn(), translate: vi.fn(), rotate: vi.fn() } as unknown as CanvasRenderingContext2D;
    const fakeCanvas = {
      width: 0,
      height: 0,
      getContext: vi.fn(() => fakeCtx),
      toBlob,
    } as unknown as HTMLCanvasElement;
    vi.spyOn(document, "createElement").mockImplementation(((tag: string) => {
      if (tag === "canvas") return fakeCanvas;
      return {} as HTMLElement;
    }) as typeof document.createElement);

    await cropToBlob(
      {} as HTMLImageElement,
      { x: 0, y: 0, width: 10, height: 10 },
      0,
      { mimeType: "image/webp" },
    );

    expect(toBlob).toHaveBeenCalledWith(expect.any(Function), "image/webp", 0.95);
  });

  it("defaults to image/jpeg at 0.95 when no opts given", async () => {
    const toBlob = vi.fn((cb: (b: Blob) => void, _type?: string, _q?: number) => {
      cb(new Blob(["x"], { type: "image/jpeg" }));
    });
    const fakeCtx = { drawImage: vi.fn(), translate: vi.fn(), rotate: vi.fn() } as unknown as CanvasRenderingContext2D;
    const fakeCanvas = {
      width: 0,
      height: 0,
      getContext: vi.fn(() => fakeCtx),
      toBlob,
    } as unknown as HTMLCanvasElement;
    vi.spyOn(document, "createElement").mockImplementation(((tag: string) => {
      if (tag === "canvas") return fakeCanvas;
      return {} as HTMLElement;
    }) as typeof document.createElement);

    await cropToBlob({} as HTMLImageElement, { x: 0, y: 0, width: 10, height: 10 }, 0);

    expect(toBlob).toHaveBeenCalledWith(expect.any(Function), "image/jpeg", 0.95);
  });
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/admin && npx vitest run components/products/media/cropImage.test.ts`
Expected: FAIL — `toBlob` called with `"image/jpeg", 0.92` (or new tests error on the extra arg).

- [ ] **Step 3: Implement the opts param**

In `apps/admin/components/products/media/cropImage.ts`, change the `cropToBlob` signature and the `toBlob` call:

```ts
export interface CropOutputOptions {
  mimeType?: string;
  quality?: number;
}

export async function cropToBlob(
  image: HTMLImageElement,
  box: CropBox,
  rotationDeg: number,
  opts: CropOutputOptions = {},
): Promise<Blob> {
  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("2d context unavailable");

  const sx = Math.round(box.x);
  const sy = Math.round(box.y);
  const sw = Math.round(box.width);
  const sh = Math.round(box.height);

  canvas.width = sw;
  canvas.height = sh;

  if (rotationDeg !== 0) {
    ctx.translate(sw / 2, sh / 2);
    ctx.rotate((rotationDeg * Math.PI) / 180);
    ctx.translate(-sw / 2, -sh / 2);
  }

  ctx.drawImage(image, sx, sy, sw, sh, 0, 0, sw, sh);

  const mimeType = opts.mimeType ?? "image/jpeg";
  const quality = opts.quality ?? 0.95;

  return new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (b) => (b ? resolve(b) : reject(new Error("canvas.toBlob returned null"))),
      mimeType,
      quality,
    );
  });
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/admin && npx vitest run components/products/media/cropImage.test.ts`
Expected: PASS (all cases, including the pre-existing ones).

- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/media/cropImage.ts apps/admin/components/products/media/cropImage.test.ts
git commit -m "feat(admin): cropToBlob preserves source format at quality 0.95"
```

---

### Task 3: Frontend — media resolution & MIME helpers

Small pure module: the min-resolution constant + short-edge check, a file-dimension reader, and a URL→MIME inference used by the add and recrop paths.

**Files:**
- Create: `apps/admin/components/products/media/mediaResolution.ts`
- Test: `apps/admin/components/products/media/mediaResolution.test.ts` (new)

**Interfaces:**
- Produces:
  - `MIN_SHORT_EDGE_PX = 1000`
  - `MIN_RESOLUTION_WARNING: string` (advisory copy)
  - `belowMinShortEdge(width: number, height: number): boolean`
  - `inferMimeFromUrl(url: string): "image/png" | "image/webp" | "image/jpeg"`
  - `readFileDimensions(file: File): Promise<{ width: number; height: number }>`

- [ ] **Step 1: Write the failing test**

Create `apps/admin/components/products/media/mediaResolution.test.ts`:

```ts
import { describe, it, expect, vi } from "vitest";
import {
  MIN_SHORT_EDGE_PX,
  belowMinShortEdge,
  inferMimeFromUrl,
  readFileDimensions,
} from "./mediaResolution";

describe("belowMinShortEdge", () => {
  it("is false when the short edge meets the threshold", () => {
    expect(belowMinShortEdge(4000, MIN_SHORT_EDGE_PX)).toBe(false);
    expect(belowMinShortEdge(MIN_SHORT_EDGE_PX, MIN_SHORT_EDGE_PX)).toBe(false);
  });

  it("is true when the short edge is under the threshold", () => {
    expect(belowMinShortEdge(4000, 800)).toBe(true);
    expect(belowMinShortEdge(600, 3000)).toBe(true);
  });
});

describe("inferMimeFromUrl", () => {
  it("maps known extensions and defaults to jpeg", () => {
    expect(inferMimeFromUrl("https://cdn/x.png")).toBe("image/png");
    expect(inferMimeFromUrl("https://cdn/x.PNG?v=2")).toBe("image/png");
    expect(inferMimeFromUrl("https://cdn/x.webp")).toBe("image/webp");
    expect(inferMimeFromUrl("https://cdn/x.jpg")).toBe("image/jpeg");
    expect(inferMimeFromUrl("https://cdn/x")).toBe("image/jpeg");
  });
});

describe("readFileDimensions", () => {
  it("resolves natural dimensions from a loaded image", async () => {
    const created: { onload?: () => void; onerror?: () => void; naturalWidth: number; naturalHeight: number; src: string } = {
      naturalWidth: 2400,
      naturalHeight: 1600,
      src: "",
    };
    const origImage = globalThis.Image;
    (globalThis as unknown as { Image: unknown }).Image = function () {
      return created;
    };
    const origCreate = URL.createObjectURL;
    const origRevoke = URL.revokeObjectURL;
    URL.createObjectURL = vi.fn(() => "blob:x");
    URL.revokeObjectURL = vi.fn();

    const file = new File(["x"], "a.jpg", { type: "image/jpeg" });
    const p = readFileDimensions(file);
    created.onload?.();
    await expect(p).resolves.toEqual({ width: 2400, height: 1600 });

    (globalThis as unknown as { Image: unknown }).Image = origImage;
    URL.createObjectURL = origCreate;
    URL.revokeObjectURL = origRevoke;
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/admin && npx vitest run components/products/media/mediaResolution.test.ts`
Expected: FAIL — module `./mediaResolution` not found.

- [ ] **Step 3: Implement the module**

Create `apps/admin/components/products/media/mediaResolution.ts`:

```ts
// Advisory resolution guard + MIME helpers shared by the add and
// recrop media paths. The guard NEVER blocks — it only warns.

export const MIN_SHORT_EDGE_PX = 1000;

export const MIN_RESOLUTION_WARNING =
  "Smaller than recommended for a crisp storefront (under 1000px). You can still use it.";

/** True when the image's shorter edge is under the recommended floor. */
export function belowMinShortEdge(width: number, height: number): boolean {
  if (!Number.isFinite(width) || !Number.isFinite(height)) return false;
  return Math.min(width, height) < MIN_SHORT_EDGE_PX;
}

/** Best-effort MIME inference from a URL or storage key extension. */
export function inferMimeFromUrl(url: string): "image/png" | "image/webp" | "image/jpeg" {
  const clean = url.split("?")[0]?.toLowerCase() ?? "";
  if (clean.endsWith(".png")) return "image/png";
  if (clean.endsWith(".webp")) return "image/webp";
  return "image/jpeg";
}

/** Reads a File's natural pixel dimensions via an off-screen Image. */
export function readFileDimensions(file: File): Promise<{ width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      const dims = { width: img.naturalWidth, height: img.naturalHeight };
      URL.revokeObjectURL(url);
      resolve(dims);
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error(`failed to read dimensions for ${file.name}`));
    };
    img.src = url;
  });
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/admin && npx vitest run components/products/media/mediaResolution.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/media/mediaResolution.ts apps/admin/components/products/media/mediaResolution.test.ts
git commit -m "feat(admin): add media resolution guard + MIME helpers"
```

---

### Task 4: Frontend — `MediaCropDialog` aspect selector, source format, min-res advisory

Add the aspect selector (default 4:5), pass the source MIME into `cropToBlob`, and show a non-blocking advisory when the crop output is under the resolution floor.

**Files:**
- Modify: `apps/admin/components/products/media/MediaCropDialog.tsx`
- Test: `apps/admin/components/products/media/MediaCropDialog.test.tsx`

**Interfaces:**
- Consumes: `cropToBlob(img, box, rotation, { mimeType, quality })` (Task 2); `belowMinShortEdge`, `MIN_RESOLUTION_WARNING` (Task 3).
- Produces: `MediaCropDialog` prop `sourceMimeType?: string` (default `"image/jpeg"`). Default `aspect` is now `4/5`. The dialog still calls `onApply(blob, box, rotation)` — unchanged signature.

- [ ] **Step 1: Write the failing tests**

Add to `apps/admin/components/products/media/MediaCropDialog.test.tsx` (inside the `describe("MediaCropDialog", …)` block). Note the existing file already mocks `./cropImage`; extend that mock at the top of the file to capture args by changing the existing `vi.mock("./cropImage", …)` to expose the mock:

Replace the existing mock block:

```ts
vi.mock("./cropImage", () => ({
  cropToBlob: vi.fn(async () => new Blob(["x"], { type: "image/jpeg" })),
  loadImage: vi.fn(async () => ({ naturalWidth: 800, naturalHeight: 600 } as HTMLImageElement)),
}));
```

with:

```ts
import { cropToBlob as mockCropToBlob } from "./cropImage";

vi.mock("./cropImage", () => ({
  cropToBlob: vi.fn(async () => new Blob(["x"], { type: "image/jpeg" })),
  loadImage: vi.fn(async () => ({ naturalWidth: 800, naturalHeight: 600 } as HTMLImageElement)),
}));
```

Then add these tests:

```ts
  it("passes sourceMimeType through to cropToBlob", async () => {
    const onApply = vi.fn();
    render(
      <MediaCropDialog
        sourceUrl="blob:test"
        sourceMimeType="image/webp"
        onApply={onApply}
        onCancel={vi.fn()}
      />,
    );
    await waitFor(() => expect(screen.getByRole("button", { name: /apply/i })).not.toBeDisabled());
    fireEvent.click(screen.getByRole("button", { name: /apply/i }));
    await waitFor(() => expect(onApply).toHaveBeenCalledTimes(1));
    expect(mockCropToBlob).toHaveBeenCalledWith(
      expect.anything(),
      { x: 10, y: 20, width: 200, height: 300 },
      0,
      { mimeType: "image/webp", quality: 0.95 },
    );
  });

  it("renders aspect controls including a default and Free option", () => {
    render(<MediaCropDialog sourceUrl="blob:test" onApply={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByRole("button", { name: /^4:5$/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^1:1$/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^3:4$/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^free$/i })).toBeInTheDocument();
  });

  it("shows a non-blocking advisory when the crop output is below the floor and still applies", async () => {
    // The mock cropper reports a 200x300 pixel box → short edge 200 < 1000.
    const onApply = vi.fn();
    render(<MediaCropDialog sourceUrl="blob:test" onApply={onApply} onCancel={vi.fn()} />);
    await waitFor(() => expect(screen.getByRole("button", { name: /apply/i })).not.toBeDisabled());
    expect(screen.getByText(/smaller than recommended/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /apply/i }));
    await waitFor(() => expect(onApply).toHaveBeenCalledTimes(1));
  });
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/admin && npx vitest run components/products/media/MediaCropDialog.test.tsx`
Expected: FAIL — no aspect buttons, no advisory text, `cropToBlob` called with 3 args (no opts).

- [ ] **Step 3: Implement the dialog changes**

In `apps/admin/components/products/media/MediaCropDialog.tsx`:

Update imports and props:

```tsx
import { cropToBlob, loadImage, type CropBox } from "./cropImage";
import { belowMinShortEdge, MIN_RESOLUTION_WARNING } from "./mediaResolution";

export interface MediaCropDialogProps {
  sourceUrl: string;
  aspect?: number;
  sourceMimeType?: string;
  onApply: (blob: Blob, box: CropBox, rotation: number) => void | Promise<void>;
  onCancel: () => void;
}

const ASPECT_OPTIONS: { label: string; value: number | undefined }[] = [
  { label: "4:5", value: 4 / 5 },
  { label: "1:1", value: 1 },
  { label: "3:4", value: 3 / 4 },
  { label: "Free", value: undefined },
];
```

Change the function signature default and add aspect state (default 4:5):

```tsx
export function MediaCropDialog({
  sourceUrl,
  aspect,
  sourceMimeType = "image/jpeg",
  onApply,
  onCancel,
}: MediaCropDialogProps): React.ReactElement {
  const [selectedAspect, setSelectedAspect] = useState<number | undefined>(
    aspect ?? 4 / 5,
  );
  const [crop, setCrop] = useState<{ x: number; y: number }>({ x: 0, y: 0 });
  const [zoom, setZoom] = useState<number>(1);
  const [rotation, setRotation] = useState<number>(0);
  const [pixelCrop, setPixelCrop] = useState<CropBox | null>(null);
  const [busy, setBusy] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
```

Derive the advisory from the current pixel crop (non-blocking):

```tsx
  const lowResWarning =
    pixelCrop && belowMinShortEdge(pixelCrop.width, pixelCrop.height)
      ? MIN_RESOLUTION_WARNING
      : null;
```

Pass source format into `cropToBlob` inside `handleApply`:

```tsx
      const blob = await cropToBlob(img, pixelCrop, rotation, {
        mimeType: sourceMimeType,
        quality: 0.95,
      });
```

Pass `selectedAspect` to the `Cropper`:

```tsx
        <Cropper
          image={sourceUrl}
          crop={crop}
          zoom={zoom}
          rotation={rotation}
          aspect={selectedAspect}
          onCropChange={setCrop}
          onZoomChange={setZoom}
          onRotationChange={setRotation}
          onCropComplete={handleCropComplete}
        />
```

Add the aspect selector + advisory to the controls bar (in the controls `<div>` that holds Zoom/Rotate, before the `{error …}` block):

```tsx
        <div className="flex items-center gap-2 text-sm" role="group" aria-label="Aspect ratio">
          <span className="text-[var(--ink-700)]">Aspect</span>
          {ASPECT_OPTIONS.map((opt) => {
            const isActive = selectedAspect === opt.value;
            return (
              <button
                key={opt.label}
                type="button"
                aria-pressed={isActive}
                onClick={() => setSelectedAspect(opt.value)}
                className={`rounded-md border px-3 py-1 focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] ${
                  isActive
                    ? "border-[var(--moss-700)] bg-[var(--moss-700)] text-[var(--paper-200)]"
                    : "border-[var(--ink-200)] text-[var(--ink-700)]"
                }`}
              >
                {opt.label}
              </button>
            );
          })}
        </div>
        {lowResWarning ? (
          <p role="status" className="text-sm text-[var(--warning)]">
            {lowResWarning}
          </p>
        ) : null}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/admin && npx vitest run components/products/media/MediaCropDialog.test.tsx`
Expected: PASS (all cases, including the pre-existing ones).

- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/media/MediaCropDialog.tsx apps/admin/components/products/media/MediaCropDialog.test.tsx
git commit -m "feat(admin): crop dialog aspect selector, source format, min-res advisory"
```

---

### Task 5: Frontend — add flow uploads the pristine original; recrop sends content_type

Rewrite the add path so dropped files upload untouched (no forced crop, no queue). Cropping stays available only through the per-card action, which now sends the source `content_type` and the source MIME to the dialog.

**Files:**
- Modify: `apps/admin/lib/api/marketplace-api.ts` (`RecropMediaInput`, ~line 683)
- Modify: `apps/admin/components/products/media/MediaUploader.tsx` (add the `warning?` field to the item type only — rendering lands in Task 6)
- Modify: `apps/admin/components/products/form/MediaTab.tsx`
- Test: `apps/admin/components/products/form/MediaTab.test.tsx`

**Interfaces:**
- Consumes: `uploadMediaFile` (unchanged — uploads the raw `File`), `recropMedia` (now accepts `content_type`), `inferMimeFromUrl` + `readFileDimensions` + `belowMinShortEdge` + `MIN_RESOLUTION_WARNING` (Task 3), `MediaCropDialog` `sourceMimeType` prop (Task 4).
- Produces: add flow calls `uploadOne(rawFile, position, progressId)` directly; `CropTarget` loses the `"fresh"` mode and `file` field; `handleCropApply` keeps only the recrop branch. `RecropMediaInput` gains `content_type?: "image/png" | "image/jpeg" | "image/webp"`. `MediaUploaderProgressItem` gains `warning?: string` (set here, rendered in Task 6).

- [ ] **Step 1: Extend the recrop client input type**

In `apps/admin/lib/api/marketplace-api.ts`, add `content_type` to `RecropMediaInput` (the interface just above `RecropMediaResult`, ~line 683):

```ts
export interface RecropMediaInput {
  crop_box: CropBox;
  rotation?: number;
  filename?: string;
  content_type?: "image/png" | "image/jpeg" | "image/webp";
}
```

(No change needed to `recropMedia` itself — it already `JSON.stringify(body)`.)

- [ ] **Step 1b: Add the `warning?` field to the uploader item type**

In `apps/admin/components/products/media/MediaUploader.tsx`, add `warning?: string` to `MediaUploaderProgressItem` so the add flow can attach the advisory (rendering is added in Task 6):

```tsx
export interface MediaUploaderProgressItem {
  id: string;
  filename: string;
  percent: number;
  status: "uploading" | "done" | "error";
  error?: string;
  warning?: string;
}
```

- [ ] **Step 2: Rewrite the MediaTab unit tests (failing)**

Replace `apps/admin/components/products/form/MediaTab.test.tsx` in full:

```tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { FormProvider, useForm } from "react-hook-form";
import type { ReactElement } from "react";
import { MediaTab } from "./MediaTab";
import type { ProductFormValues } from "@/lib/validation/product-form";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

// Auto-applying crop dialog stub (used only by the re-crop path now).
vi.mock("@/components/products/media/MediaCropDialog", () => ({
  MediaCropDialog: ({
    onApply,
  }: {
    onApply: (
      blob: Blob,
      box: { x: number; y: number; width: number; height: number },
      rotation: number,
    ) => void | Promise<void>;
  }): ReactElement => (
    <button
      type="button"
      data-testid="auto-apply-crop"
      onClick={() =>
        void onApply(new Blob(["cropped"], { type: "image/jpeg" }), { x: 0, y: 0, width: 100, height: 100 }, 0)
      }
    >
      auto apply
    </button>
  ),
}));

// Deterministic dimensions so the add-flow min-res guard is exercised
// without a real image decode. 2400x1600 → above the 1000px floor.
vi.mock("@/components/products/media/mediaResolution", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/products/media/mediaResolution")>();
  return {
    ...actual,
    readFileDimensions: vi.fn(async () => ({ width: 2400, height: 1600 })),
  };
});

function makeMedia(id: string, position: number): AdminMediaResponse {
  return {
    id,
    url: `https://cdn.test/${id}.jpg`,
    storage_key: `products/${id}`,
    alt: `alt ${id}`,
    position,
    media_type: "image",
    variant_id: null,
    width: 800,
    height: 800,
    bytes: 1024,
  };
}

interface HarnessProps {
  initialMedia?: AdminMediaResponse[];
  uploadMediaFile: (...args: unknown[]) => unknown;
  recropMedia: (...args: unknown[]) => unknown;
  updateMedia: (...args: unknown[]) => unknown;
  deleteMedia: (...args: unknown[]) => unknown;
  putBlob: (...args: unknown[]) => unknown;
}

function Harness(props: HarnessProps): ReactElement {
  const methods = useForm<ProductFormValues>({
    defaultValues: {
      media: (props.initialMedia ?? []).map((m) => ({
        id: m.id,
        url: m.url,
        alt: m.alt ?? "",
        position: m.position,
        variant_id: m.variant_id,
        storage_key: m.storage_key,
        gcs_path_original: "",
      })),
    } as Partial<ProductFormValues> as ProductFormValues,
  });
  return (
    <FormProvider {...methods}>
      <MediaTab
        storeId="store_1"
        productId="prod_1"
        session={{ userId: "u", tenantId: "t" }}
        uploadMediaFile={props.uploadMediaFile as never}
        recropMedia={props.recropMedia as never}
        updateMedia={props.updateMedia as never}
        deleteMedia={props.deleteMedia as never}
        putBlob={props.putBlob as never}
      />
    </FormProvider>
  );
}

describe("MediaTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("uploads the pristine original directly on add without opening a crop dialog", async () => {
    const uploaded = makeMedia("new1", 0);
    const uploadMediaFile = vi.fn(async () => uploaded);

    render(
      <Harness
        uploadMediaFile={uploadMediaFile}
        recropMedia={vi.fn()}
        updateMedia={vi.fn()}
        deleteMedia={vi.fn()}
        putBlob={vi.fn()}
      />,
    );

    const input = screen.getByLabelText(/drop images/i) as HTMLInputElement;
    const file = new File(["x"], "shot.jpg", { type: "image/jpeg" });
    await act(async () => {
      fireEvent.change(input, { target: { files: [file] } });
    });

    // No crop dialog on add.
    expect(screen.queryByTestId("auto-apply-crop")).not.toBeInTheDocument();
    // The raw File is uploaded (first positional arg), not a derived blob.
    await waitFor(() => expect(uploadMediaFile).toHaveBeenCalledTimes(1));
    const firstArg = (uploadMediaFile.mock.calls[0]?.[0] ?? {}) as { file?: File };
    expect(firstArg.file).toBe(file);
    await waitFor(() => expect(screen.getByAltText("alt new1")).toBeInTheDocument());
  });

  it("uploads multiple dropped files directly", async () => {
    const uploadMediaFile = vi
      .fn()
      .mockResolvedValueOnce(makeMedia("a", 0))
      .mockResolvedValueOnce(makeMedia("b", 1));

    render(
      <Harness
        uploadMediaFile={uploadMediaFile}
        recropMedia={vi.fn()}
        updateMedia={vi.fn()}
        deleteMedia={vi.fn()}
        putBlob={vi.fn()}
      />,
    );

    const input = screen.getByLabelText(/drop images/i) as HTMLInputElement;
    const f1 = new File(["a"], "a.jpg", { type: "image/jpeg" });
    const f2 = new File(["b"], "b.jpg", { type: "image/jpeg" });
    await act(async () => {
      fireEvent.change(input, { target: { files: [f1, f2] } });
    });

    expect(screen.queryByTestId("auto-apply-crop")).not.toBeInTheDocument();
    await waitFor(() => expect(uploadMediaFile).toHaveBeenCalledTimes(2));
  });

  it("delete action removes the card and calls deleteMedia", async () => {
    const initial = [makeMedia("a", 0), makeMedia("b", 1)];
    const deleteMedia = vi.fn(async () => ({ ok: true as const, data: true as const }));

    render(
      <Harness
        initialMedia={initial}
        uploadMediaFile={vi.fn()}
        recropMedia={vi.fn()}
        updateMedia={vi.fn()}
        deleteMedia={deleteMedia}
        putBlob={vi.fn()}
      />,
    );

    const buttons = screen.getAllByRole("button", { name: /image actions/i });
    fireEvent.click(buttons[0]!);
    fireEvent.click(screen.getByRole("menuitem", { name: /delete/i }));

    await waitFor(() => expect(deleteMedia).toHaveBeenCalledWith("store_1", "prod_1", "a", { userId: "u", tenantId: "t" }));
    await waitFor(() => expect(screen.queryByAltText("alt a")).not.toBeInTheDocument());
    expect(screen.getByAltText("alt b")).toBeInTheDocument();
  });

  it("crop action recrops with the source content_type then updates", async () => {
    const initial = [makeMedia("a", 0)]; // url ends in .jpg → image/jpeg
    const recropMedia = vi.fn(async () => ({
      ok: true as const,
      data: {
        source_original_url: "https://cdn.test/original.jpg",
        upload_url: "https://upload.test/put",
        new_storage_key: "products/new_key",
        expires_at: "2099-01-01T00:00:00Z",
      },
    }));
    const updateMedia = vi.fn(async () => ({ ok: true as const, data: true as const }));
    const putBlob = vi.fn(async () => ({ ok: true, status: 200 }));

    render(
      <Harness
        initialMedia={initial}
        uploadMediaFile={vi.fn()}
        recropMedia={recropMedia}
        updateMedia={updateMedia}
        deleteMedia={vi.fn()}
        putBlob={putBlob}
      />,
    );

    const buttons = screen.getAllByRole("button", { name: /image actions/i });
    fireEvent.click(buttons[0]!);
    fireEvent.click(screen.getByRole("menuitem", { name: /^crop$/i }));

    await waitFor(() => expect(recropMedia).toHaveBeenCalledTimes(1));
    // content_type inferred from the media URL (.jpg → image/jpeg).
    const recropBody = recropMedia.mock.calls[0]?.[3] as { content_type?: string };
    expect(recropBody.content_type).toBe("image/jpeg");

    await waitFor(() => expect(screen.getByTestId("auto-apply-crop")).toBeInTheDocument());
    await act(async () => {
      fireEvent.click(screen.getByTestId("auto-apply-crop"));
    });

    await waitFor(() => expect(putBlob).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(updateMedia).toHaveBeenCalledWith(
        "store_1",
        "prod_1",
        "a",
        { storage_key: "products/new_key" },
        { userId: "u", tenantId: "t" },
      ),
    );
  });
});
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd apps/admin && npx vitest run components/products/form/MediaTab.test.tsx`
Expected: FAIL — add flow still opens the dialog / uploads a blob; recrop body has no `content_type`.

- [ ] **Step 4: Rewrite the add + recrop logic in MediaTab**

In `apps/admin/components/products/form/MediaTab.tsx`:

Add imports:

```tsx
import { MediaCropDialog } from "@/components/products/media/MediaCropDialog";
import type { CropBox } from "@/components/products/media/cropImage";
import {
  belowMinShortEdge,
  inferMimeFromUrl,
  MIN_RESOLUTION_WARNING,
  readFileDimensions,
} from "@/components/products/media/mediaResolution";
```

Simplify the `CropTarget` type to the recrop-only shape:

```tsx
interface CropTarget {
  mediaId: string;
  sourceUrl: string;
  sourceMimeType: string;
  uploadUrl: string;
  newStorageKey: string;
}
```

Remove the `pendingFreshQueue` state entirely. Replace `handleFiles` so it uploads each dropped file untouched, sequentially, with a per-file min-res advisory:

```tsx
  const handleFiles = useCallback(
    (files: File[]) => {
      const start = fields.length;
      void (async () => {
        for (let i = 0; i < files.length; i += 1) {
          const file = files[i]!;
          const progressId = crypto.randomUUID();
          setProgress((p) => [
            ...p,
            { id: progressId, filename: file.name, percent: 0, status: "uploading" },
          ]);
          try {
            const dims = await readFileDimensions(file);
            if (belowMinShortEdge(dims.width, dims.height)) {
              setProgressItem(progressId, { warning: MIN_RESOLUTION_WARNING });
            }
          } catch {
            // Dimension read failed — fail open, skip the advisory.
          }
          await uploadOne(file, start + i, progressId);
        }
      })();
    },
    [fields.length, setProgressItem, uploadOne],
  );
```

Update the recrop branch of `handleAction` (`action === "crop"`) to compute and forward the source MIME:

```tsx
      if (action === "crop") {
        void (async () => {
          const sourceMimeType = inferMimeFromUrl(media.url);
          const res = await recropMedia(
            storeId,
            productId,
            media.id,
            { crop_box: { x: 0, y: 0, width: 0, height: 0 }, content_type: sourceMimeType },
            session,
          );
          if (!res.ok) return;
          setCropTarget({
            mediaId: media.id,
            sourceUrl: res.data.source_original_url,
            sourceMimeType,
            uploadUrl: res.data.upload_url,
            newStorageKey: res.data.new_storage_key,
          });
        })();
      }
```

Replace `handleCropApply` with the recrop-only version (the `"fresh"` branch is deleted):

```tsx
  const handleCropApply = useCallback(
    async (blob: Blob, _box: CropBox, _rotation: number) => {
      if (!cropTarget) return;
      const target = cropTarget;
      setCropTarget(null);
      URL.revokeObjectURL(target.sourceUrl);

      const putRes = await putBlob(target.uploadUrl, blob);
      if (!putRes.ok) return;
      const updateRes = await updateMedia(
        storeId,
        productId,
        target.mediaId,
        { storage_key: target.newStorageKey },
        session,
      );
      if (!updateRes.ok) return;
      const idx = fields.findIndex((f) => f.id === target.mediaId);
      const current = idx >= 0 ? fields[idx] : undefined;
      if (current) {
        update(idx, {
          id: current.id,
          url: current.url,
          alt: current.alt,
          position: current.position,
          variant_id: current.variant_id,
          storage_key: target.newStorageKey,
          gcs_path_original: current.gcs_path_original,
        });
      }
    },
    [cropTarget, fields, productId, putBlob, session, storeId, update, updateMedia],
  );
```

Simplify `handleCropCancel` (no queue to clear):

```tsx
  const handleCropCancel = useCallback(() => {
    if (cropTarget) URL.revokeObjectURL(cropTarget.sourceUrl);
    setCropTarget(null);
  }, [cropTarget]);
```

Pass `sourceMimeType` to the dialog in the JSX:

```tsx
      {cropTarget ? (
        <MediaCropDialog
          sourceUrl={cropTarget.sourceUrl}
          sourceMimeType={cropTarget.sourceMimeType}
          onApply={handleCropApply}
          onCancel={handleCropCancel}
        />
      ) : null}
```

Add a clarifying comment above the `gcs_path_original: ""` line in `mediaToField`:

```tsx
    // Backend is authoritative for gcs_path_original (AddMedia stamps it
    // from the finalized storage_key). This client-side "" is a form
    // placeholder only and is never sent back on save.
    gcs_path_original: "",
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd apps/admin && npx vitest run components/products/form/MediaTab.test.tsx`
Expected: PASS.

- [ ] **Step 6: Typecheck the touched files**

Run: `cd apps/admin && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add apps/admin/lib/api/marketplace-api.ts apps/admin/components/products/media/MediaUploader.tsx apps/admin/components/products/form/MediaTab.tsx apps/admin/components/products/form/MediaTab.test.tsx
git commit -m "fix(admin): upload pristine original on add; crop optional via recrop with source format"
```

---

### Task 6: Frontend — surface the add-time min-resolution advisory

Render the per-file warning the add flow now sets on progress items.

**Files:**
- Modify: `apps/admin/components/products/media/MediaUploader.tsx`
- Test: `apps/admin/components/products/media/MediaUploader.test.tsx`

**Interfaces:**
- Consumes: `MediaUploaderProgressItem.warning?: string` (field already added in Task 5); items carrying a `warning` are set by MediaTab's add flow.
- Produces: the uploader renders `item.warning` as a `role="status"` advisory line under the item row.

- [ ] **Step 1: Write the failing test**

Add to `apps/admin/components/products/media/MediaUploader.test.tsx`:

```ts
  it("renders a resolution advisory when a progress item carries a warning", () => {
    render(
      <MediaUploader
        onFiles={vi.fn()}
        progressItems={[
          {
            id: "1",
            filename: "small.jpg",
            percent: 100,
            status: "done",
            warning: "Smaller than recommended for a crisp storefront (under 1000px). You can still use it.",
          },
        ]}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(/smaller than recommended/i);
  });
```

(If the test file lacks the imports, mirror the existing ones at the top: `import { describe, it, expect, vi } from "vitest";` and `import { render, screen } from "@testing-library/react";` and `import { MediaUploader } from "./MediaUploader";`.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/admin && npx vitest run components/products/media/MediaUploader.test.tsx`
Expected: FAIL — no `role="status"` warning rendered.

- [ ] **Step 3: Render the advisory line**

The `warning?: string` field is already on `MediaUploaderProgressItem` (added in Task 5). Change each progress `<li>` to a flex-col that renders the advisory line under the row. Replace the `<li>` body:

```tsx
            <li
              key={item.id}
              className="flex flex-col gap-1 border-b border-[var(--ink-100)] py-2 text-sm"
            >
              <div className="flex items-center gap-3">
                <span className="flex-1 truncate text-[var(--ink-900)]">{item.filename}</span>
                <span className="w-40 h-1 overflow-hidden rounded-full bg-[var(--ink-100)]">
                  <span
                    className={`block h-full ${
                      item.status === "error"
                        ? "bg-[var(--danger)]"
                        : "bg-[var(--moss-700)]"
                    }`}
                    style={{ width: `${item.percent}%` }}
                  />
                </span>
                <span className="w-12 text-right text-xs uppercase tracking-widest text-[var(--ink-500)]">
                  {item.status === "done" ? "Done" : item.status === "error" ? "Error" : `${item.percent}%`}
                </span>
              </div>
              {item.warning ? (
                <span role="status" className="text-xs text-[var(--warning)]">
                  {item.warning}
                </span>
              ) : null}
            </li>
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/admin && npx vitest run components/products/media/MediaUploader.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/media/MediaUploader.tsx apps/admin/components/products/media/MediaUploader.test.tsx
git commit -m "feat(admin): surface min-resolution advisory in media uploader"
```

---

### Task 7: E2E — add uploads directly; crop exercised via the card menu

The current E2E encodes the buggy "crop dialog on add" behavior. Update it so add uploads without a dialog, and the crop dialog is exercised only through the overflow menu.

**Files:**
- Modify: `apps/admin/tests/e2e/products-media-flow.spec.ts`

**Interfaces:**
- Consumes: `applyCropDialog(page)` helper (unchanged; now used only for the menu crop path). `signInAndSeedProduct`, `selectTab` (unchanged).

- [ ] **Step 1: Update the doc comment and the first test**

In `apps/admin/tests/e2e/products-media-flow.spec.ts`, update the header comment bullet that says "the crop dialog opens automatically per drop; accept each," to:

```
 *   - upload two JPEGs through the hidden MediaUploader input,
 *   - they upload directly (NO crop dialog on add — the original is kept),
 *   - MediaGrid renders two MediaCards,
 *   - open the overflow menu and crop one image (recrop flow),
```

In the first test (`"upload, crop, reorder to primary, round-trip"`), replace the upload+apply block:

```ts
    // 1. Upload two images via the hidden file input. Add uploads the
    // pristine original directly — no crop dialog appears.
    await page.locator("#media-uploader-input").setInputFiles([RED, BLUE]);

    // 2. Two MediaCards in the grid (no dialog to dismiss).
    const cardImages = page.getByRole("list").locator("img");
    await expect(cardImages).toHaveCount(2, { timeout: 15_000 });
    await expect(page.getByRole("dialog", { name: /crop image/i })).toHaveCount(0);
```

(The subsequent "Re-crop the first card via its overflow menu" block that calls `applyCropDialog(page)` stays as-is.)

- [ ] **Step 2: Update the second test**

In `"re-crop hits POST /media/:id/recrop and preserves original"`, replace the initial upload+apply:

```ts
    // Upload one image — it uploads directly, no crop dialog on add.
    await page.locator("#media-uploader-input").setInputFiles([RED]);

    const cardImages = page.getByRole("list").locator("img");
    await expect(cardImages).toHaveCount(1, { timeout: 15_000 });
```

(The rest — save, reload, overflow-menu crop, `recropPromise`, `applyCropDialog(page)` — stays as-is.)

- [ ] **Step 3: Lint/typecheck the spec**

Run: `cd apps/admin && npx tsc --noEmit`
Expected: no errors. (The E2E requires a live stack to run fully; CI executes it. Local typecheck is the gate here.)

- [ ] **Step 4: Commit**

```bash
git add apps/admin/tests/e2e/products-media-flow.spec.ts
git commit -m "test(admin): e2e — add uploads directly, crop via card menu"
```

---

## Final verification

- [ ] **Run the full admin media unit suite:**

Run: `cd apps/admin && npx vitest run components/products/media components/products/form/MediaTab.test.tsx`
Expected: all PASS.

- [ ] **Typecheck the admin app:**

Run: `cd apps/admin && npx tsc --noEmit`
Expected: no errors.

- [ ] **Build + vet the backend:**

Run: `cd services/marketplace-api && go build ./... && go vet ./internal/...`
Expected: no output.

- [ ] **Backend recrop integration tests (if a test DB is available locally):**

Run: `cd services/marketplace-api && go test -tags=integration ./internal/product/... -run TestIntegration_RecropMedia`
Expected: PASS. (Otherwise rely on CI.)

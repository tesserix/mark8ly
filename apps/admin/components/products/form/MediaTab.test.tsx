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

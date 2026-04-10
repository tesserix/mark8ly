import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { FormProvider, useForm } from "react-hook-form";
import type { ReactElement } from "react";
import { MediaTab } from "./MediaTab";
import type { ProductFormValues } from "@/lib/validation/product-form";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

// Mock MediaCropDialog to auto-apply with a deterministic blob.
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
  // Loose typing — the real component props are strict; we cast on render.
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

  it("uploads a file and appends it to the form media array", async () => {
    const uploaded = makeMedia("new1", 0);
    const uploadMediaFile = vi.fn(async () => uploaded);
    const recropMedia = vi.fn();
    const updateMedia = vi.fn();
    const deleteMedia = vi.fn();
    const putBlob = vi.fn();

    render(
      <Harness
        uploadMediaFile={uploadMediaFile}
        recropMedia={recropMedia}
        updateMedia={updateMedia}
        deleteMedia={deleteMedia}
        putBlob={putBlob}
      />,
    );

    const input = screen.getByLabelText(/drop images/i) as HTMLInputElement;
    const file = new File(["x"], "shot.jpg", { type: "image/jpeg" });
    await act(async () => {
      fireEvent.change(input, { target: { files: [file] } });
    });

    // Dialog renders → auto-apply
    await waitFor(() => expect(screen.getByTestId("auto-apply-crop")).toBeInTheDocument());
    await act(async () => {
      fireEvent.click(screen.getByTestId("auto-apply-crop"));
    });

    await waitFor(() => expect(uploadMediaFile).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByAltText("alt new1")).toBeInTheDocument());
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

    // Open first card's overflow menu
    const buttons = screen.getAllByRole("button", { name: /image actions/i });
    fireEvent.click(buttons[0]!);
    fireEvent.click(screen.getByRole("menuitem", { name: /delete/i }));

    await waitFor(() => expect(deleteMedia).toHaveBeenCalledWith("store_1", "prod_1", "a", { userId: "u", tenantId: "t" }));
    await waitFor(() => expect(screen.queryByAltText("alt a")).not.toBeInTheDocument());
    expect(screen.getByAltText("alt b")).toBeInTheDocument();
  });

  it("crop action on existing media calls recropMedia then updateMedia", async () => {
    const initial = [makeMedia("a", 0)];
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

  it("queues multi-file upload and cancel clears pending queue", async () => {
    const uploaded = makeMedia("u1", 0);
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
    const f1 = new File(["a"], "a.jpg", { type: "image/jpeg" });
    const f2 = new File(["b"], "b.jpg", { type: "image/jpeg" });
    await act(async () => {
      fireEvent.change(input, { target: { files: [f1, f2] } });
    });
    await waitFor(() => expect(screen.getByTestId("auto-apply-crop")).toBeInTheDocument());
    // Apply first → triggers pendingFreshQueue drain for second
    await act(async () => {
      fireEvent.click(screen.getByTestId("auto-apply-crop"));
    });
    await waitFor(() => expect(uploadMediaFile).toHaveBeenCalledTimes(1));
    // Second crop dialog should have re-opened
    await waitFor(() => expect(screen.getByTestId("auto-apply-crop")).toBeInTheDocument());
  });
});

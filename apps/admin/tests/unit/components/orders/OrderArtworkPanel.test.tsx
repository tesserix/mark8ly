/**
 * OrderArtworkPanel unit tests (#968).
 *
 * The panel is the merchant's whole view of a custom order, so these
 * tests are about the ways it could mislead someone into shipping the
 * wrong thing:
 *
 *   1. it must disappear entirely for ordinary orders, not render an
 *      empty "Personalisation" heading that invites a hunt for files
 *      that do not exist;
 *   2. it must not offer a download for a text answer, because a dead
 *      button reads as a missing file;
 *   3. it must mint links on click and never on render — each one is
 *      short-lived and audited;
 *   4. a failed fetch must say so. A silent failure is the one outcome
 *      that ends with a customer receiving a blank mug.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import React from "react";

import { OrderArtworkPanel } from "@/components/orders/OrderArtworkPanel";
import type {
  AdminOrderItem,
  AdminOrderPersonalisation,
} from "@/lib/api/marketplace-api";

const STORE_ID = "11111111-1111-1111-1111-111111111111";
const ORDER_ID = "22222222-2222-2222-2222-222222222222";
const IMAGE_ID = "33333333-3333-3333-3333-333333333333";
const TEXT_ID = "44444444-4444-4444-4444-444444444444";

const toastError = vi.fn();
const toastInfo = vi.fn();

vi.mock("@/components/feedback/Toaster", () => ({
  useToast: () => ({
    toast: {
      error: toastError,
      info: toastInfo,
      success: vi.fn(),
      warning: vi.fn(),
    },
  }),
}));

function imageAnswer(
  over: Partial<AdminOrderPersonalisation> = {},
): AdminOrderPersonalisation {
  return {
    id: IMAGE_ID,
    field_key: "photo",
    field_label: "Your photo",
    kind: "image",
    price_delta: "10.00",
    position: 0,
    has_artwork: true,
    original_filename: "nana.jpg",
    content_type: "image/jpeg",
    size_bytes: 204800,
    reference: IMAGE_ID.slice(0, 8),
    ...over,
  };
}

function textAnswer(
  over: Partial<AdminOrderPersonalisation> = {},
): AdminOrderPersonalisation {
  return {
    id: TEXT_ID,
    field_key: "name",
    field_label: "Name to engrave",
    kind: "text",
    text_value: "Asha",
    price_delta: "0.00",
    position: 1,
    has_artwork: false,
    reference: TEXT_ID.slice(0, 8),
    ...over,
  };
}

function item(
  personalisation?: AdminOrderPersonalisation[],
): AdminOrderItem {
  return {
    id: "item-1",
    title_snapshot: "Custom figurine",
    sku_snapshot: "FIG-1",
    unit_price: "80.00",
    quantity: 1,
    line_total: "80.00",
    currency_code: "USD",
    personalisation,
  };
}

let openSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  toastError.mockReset();
  toastInfo.mockReset();
  openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
});

afterEach(() => {
  openSpy.mockRestore();
  vi.unstubAllGlobals();
});

describe("OrderArtworkPanel", () => {
  it("renders nothing for an order with no personalisation", () => {
    const { container } = render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[item(), item(undefined)]}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the buyer's text verbatim and offers no download for it", () => {
    render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[item([textAnswer()])]}
      />,
    );
    expect(screen.getByText("Name to engrave")).toBeInTheDocument();
    expect(screen.getByText("Asha")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /download original/i }),
    ).not.toBeInTheDocument();
  });

  it("does not fetch any link on render", () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[item([imageAnswer(), textAnswer()])]}
      />,
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("shows the filename, reference and size for an upload", () => {
    render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[item([imageAnswer()])]}
      />,
    );
    expect(screen.getByText("nana.jpg")).toBeInTheDocument();
    expect(
      screen.getByText(
        (t) => t.includes(`Ref ${IMAGE_ID.slice(0, 8)}`) && t.includes("200 KB"),
      ),
    ).toBeInTheDocument();
  });

  it("mints a link on click and opens it", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        personalisation_id: IMAGE_ID,
        reference: IMAGE_ID.slice(0, 8),
        filename: "nana.jpg",
        url: "https://signed.example/original.jpg",
        expires_at: new Date().toISOString(),
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[item([imageAnswer(), textAnswer()])]}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: /download original/i }),
    );

    await waitFor(() => expect(openSpy).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      `/api/admin/stores/${STORE_ID}/orders/${ORDER_ID}/personalisations/${IMAGE_ID}/download`,
      { cache: "no-store" },
    );
    expect(openSpy).toHaveBeenCalledWith(
      "https://signed.example/original.jpg",
      "_blank",
      "noopener,noreferrer",
    );
    expect(toastError).not.toHaveBeenCalled();
  });

  it("tells the merchant when the link cannot be minted", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: false, status: 404, json: async () => ({}) }),
    );

    render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[item([imageAnswer()])]}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: /download original/i }),
    );

    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));
    expect(openSpy).not.toHaveBeenCalled();
    // The copy has to stop someone mid-fulfilment, not just log.
    expect(toastError.mock.calls[0]?.[1]).toMatch(/before shipping this order/i);
  });

  it("distinguishes a missing private bucket from a missing file", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: false, status: 501, json: async () => ({}) }),
    );

    render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[item([imageAnswer()])]}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: /download original/i }),
    );

    await waitFor(() => expect(toastError).toHaveBeenCalledTimes(1));
    expect(toastError.mock.calls[0]?.[1]).toMatch(/not configured/i);
  });

  it("offers download-all only when there is more than one file", () => {
    const { rerender } = render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[item([imageAnswer(), textAnswer()])]}
      />,
    );
    expect(
      screen.queryByRole("button", { name: /download all/i }),
    ).not.toBeInTheDocument();

    rerender(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[
          item([imageAnswer(), textAnswer()]),
          {
            ...item([imageAnswer({ id: "55555555-5555-5555-5555-555555555555" })]),
            id: "item-2",
          },
        ]}
      />,
    );
    expect(
      screen.getByRole("button", { name: /download all \(2\)/i }),
    ).toBeInTheDocument();
  });

  it("opens every link from download-all", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          artwork: [
            { personalisation_id: "a", url: "https://signed.example/a.jpg" },
            { personalisation_id: "b", url: "https://signed.example/b.jpg" },
          ],
        }),
      }),
    );

    render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[
          item([imageAnswer()]),
          {
            ...item([imageAnswer({ id: "55555555-5555-5555-5555-555555555555" })]),
            id: "item-2",
          },
        ]}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /download all/i }));

    await waitFor(() => expect(openSpy).toHaveBeenCalledTimes(2));
  });

  it("orders answers by position, not by array order", () => {
    render(
      <OrderArtworkPanel
        storeId={STORE_ID}
        orderId={ORDER_ID}
        items={[
          item([
            textAnswer({ position: 5, field_label: "Last question" }),
            imageAnswer({ position: 1, field_label: "First question" }),
          ]),
        ]}
      />,
    );
    const labels = screen
      .getAllByRole("term")
      .map((el) => el.textContent?.trim());
    expect(labels).toEqual(["First question", "Last question"]);
  });
});

import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "./route";

function photon(features: Array<Record<string, string>>) {
  return new Response(
    JSON.stringify({ features: features.map((properties) => ({ properties })) }),
    { status: 200 },
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("GET /api/locations/autocomplete", () => {
  it("asks Photon for the store's country rather than filtering the world's top eight", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      photon([
        { street: "Tagu Place", city: "Sydney", state: "New South Wales", postcode: "2148", country: "Australia", countrycode: "AU" },
      ]),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await GET(new Request("http://x/api/locations/autocomplete?q=5+Tagu+place&country=au"));
    const body = (await res.json()) as { data: Array<{ city: string; country: string }> };

    const asked = new URL(fetchMock.mock.calls[0]![0] as string);
    expect(asked.searchParams.get("countrycode")).toBe("AU");
    expect(body.data).toEqual([expect.objectContaining({ city: "Sydney", country: "AU" })]);
  });

  it("still drops results from another country if Photon returns any", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        photon([{ street: "5th Avenue", city: "Taguig", country: "Philippines", countrycode: "PH" }]),
      ),
    );

    const res = await GET(new Request("http://x/api/locations/autocomplete?q=5+Tagu&country=AU"));
    expect(((await res.json()) as { data: unknown[] }).data).toEqual([]);
  });

  it("does not restrict the search when no country is given", async () => {
    const fetchMock = vi.fn().mockResolvedValue(photon([]));
    vi.stubGlobal("fetch", fetchMock);

    await GET(new Request("http://x/api/locations/autocomplete?q=5+Tagu+place"));

    const asked = new URL(fetchMock.mock.calls[0]![0] as string);
    expect(asked.searchParams.has("countrycode")).toBe(false);
  });
});

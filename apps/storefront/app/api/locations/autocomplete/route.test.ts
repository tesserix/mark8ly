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

    const asked = new URL(String(fetchMock.mock.calls[0]![0]));
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

  it("gives the suburb, not the metro area, for an Australian address", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        photon([{ name: "Tagu Place", district: "Kings Park", city: "Sydney", state: "New South Wales", postcode: "2148", countrycode: "AU" }]),
      ),
    );

    const res = await GET(new Request("http://x/api/locations/autocomplete?q=5+Tagu+pl&country=AU"));
    const [s] = ((await res.json()) as { data: Array<{ city: string }> }).data;

    expect(s?.city).toBe("Kings Park");
  });

  it("keeps the city for an Indian address, where district is a ward", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        photon([{ name: "Linking Road", district: "H/W Ward", city: "Mumbai", state: "Maharashtra", countrycode: "IN" }]),
      ),
    );

    const res = await GET(new Request("http://x/api/locations/autocomplete?q=Linking+Road&country=IN"));
    const [s] = ((await res.json()) as { data: Array<{ city: string }> }).data;

    expect(s?.city).toBe("Mumbai");
  });

  it("keeps the house number the shopper typed when Photon only knows the street", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        photon([
          { name: "Tagu Place", district: "Kings Park", city: "Sydney", countrycode: "AU" },
          { name: "Platform 8", district: "Richmond", city: "Melbourne", countrycode: "AU" },
          { housenumber: "7", street: "Tagu Place", district: "Kings Park", countrycode: "AU" },
        ]),
      ),
    );

    const res = await GET(new Request("http://x/api/locations/autocomplete?q=5+Tagu+pl&country=AU"));
    const lines = ((await res.json()) as { data: Array<{ line1: string }> }).data.map((s) => s.line1);

    expect(lines).toEqual(["5 Tagu Place", "Platform 8", "7 Tagu Place"]);
  });

  it("does not restrict the search when no country is given", async () => {
    const fetchMock = vi.fn().mockResolvedValue(photon([]));
    vi.stubGlobal("fetch", fetchMock);

    await GET(new Request("http://x/api/locations/autocomplete?q=5+Tagu+place"));

    const asked = new URL(String(fetchMock.mock.calls[0]![0]));
    expect(asked.searchParams.has("countrycode")).toBe(false);
  });
});

describe("GET /api/locations/autocomplete with Geoapify", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("uses Geoapify when the key is set, and reads its suburb as the suburb", async () => {
    vi.stubEnv("GEOAPIFY_API_KEY", "k");
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          results: [{ housenumber: "5", street: "Tagu Place", suburb: "Kings Park", city: "Sydney", state: "New South Wales", postcode: "2148", country: "Australia", country_code: "au" }],
        }),
        { status: 200 },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await GET(new Request("http://x/api/locations/autocomplete?q=5+Tagu+pl&country=AU"));
    const [s] = ((await res.json()) as { data: Array<{ line1: string; city: string; country: string }> }).data;

    const asked = new URL(String(fetchMock.mock.calls[0]![0]));
    expect(asked.host).toBe("api.geoapify.com");
    expect(asked.searchParams.get("filter")).toBe("countrycode:au");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(s).toEqual(expect.objectContaining({ line1: "5 Tagu Place", city: "Kings Park", country: "AU" }));
  });

  it("falls back to Photon when Geoapify refuses, rather than showing nothing", async () => {
    vi.stubEnv("GEOAPIFY_API_KEY", "k");
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response("quota", { status: 429 }))
      .mockResolvedValueOnce(photon([{ name: "Tagu Place", district: "Kings Park", city: "Sydney", countrycode: "AU" }]));
    vi.stubGlobal("fetch", fetchMock);

    const res = await GET(new Request("http://x/api/locations/autocomplete?q=5+Tagu+pl&country=AU"));
    const [s] = ((await res.json()) as { data: Array<{ city: string }> }).data;

    expect(new URL(String(fetchMock.mock.calls[1]![0])).host).toBe("photon.komoot.io");
    expect(s?.city).toBe("Kings Park");
  });

  it("collapses the duplicate rows OSM keeps for one street", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        photon([
          { name: "5th Avenue", city: "Taguig", postcode: "1634", countrycode: "PH", country: "Philippines" },
          { name: "5th Avenue", city: "Taguig", postcode: "1634", countrycode: "PH", country: "Philippines" },
        ]),
      ),
    );

    const res = await GET(new Request("http://x/api/locations/autocomplete?q=5th+Ave"));
    expect(((await res.json()) as { data: unknown[] }).data).toHaveLength(1);
  });
});

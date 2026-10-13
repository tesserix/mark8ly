// GET /api/locations/autocomplete?q=...&country=...
//
// Address suggestions for the shopper's address book. Geoapify answers when
// GEOAPIFY_API_KEY is set (~200 ms); otherwise, or if Geoapify fails,
// public Photon does (free, 3–5 s). Both are OpenStreetMap data, so they
// are normalised to Photon's field names and share everything below.
// The key never reaches the browser — this route is the only caller.

import { NextResponse } from "next/server";

export const dynamic = "force-dynamic";

const USER_AGENT = "mark8ly-storefront (+https://mark8ly.com)";

interface PhotonProperties {
  name?: string;
  street?: string;
  housenumber?: string;
  postcode?: string;
  city?: string;
  district?: string;
  state?: string;
  country?: string;
  countrycode?: string;
}

interface PhotonResponse {
  features?: Array<{ properties: PhotonProperties }>;
}

interface GeoapifyResult {
  name?: string;
  street?: string;
  housenumber?: string;
  postcode?: string;
  suburb?: string;
  district?: string;
  city?: string;
  state?: string;
  country?: string;
  country_code?: string;
}

interface GeoapifyResponse {
  results?: GeoapifyResult[];
}

interface AddressSuggestion {
  description: string;
  line1: string;
  city: string;
  region: string;
  postal: string;
  country: string;
}

// Geoapify's `suburb` is what Photon calls `district` (Kings Park, Bondi);
// map it there so buildCity treats both providers the same.
function fromGeoapify(r: GeoapifyResult): PhotonProperties {
  return {
    name: r.name,
    street: r.street,
    housenumber: r.housenumber,
    postcode: r.postcode,
    city: r.city,
    district: r.suburb ?? r.district,
    state: r.state,
    country: r.country,
    countrycode: r.country_code,
  };
}

async function searchGeoapify(
  q: string,
  country: string,
  apiKey: string,
): Promise<PhotonProperties[] | null> {
  const url = new URL("https://api.geoapify.com/v1/geocode/autocomplete");
  url.searchParams.set("text", q);
  url.searchParams.set("format", "json");
  url.searchParams.set("limit", "8");
  url.searchParams.set("lang", "en");
  if (country) url.searchParams.set("filter", `countrycode:${country.toLowerCase()}`);
  url.searchParams.set("apiKey", apiKey);
  try {
    const res = await fetch(url, {
      headers: { "User-Agent": USER_AGENT },
      cache: "no-store",
      signal: AbortSignal.timeout(3000),
    });
    // null, not [], so a quota or key failure falls back to Photon rather
    // than looking like "no such address".
    if (!res.ok) return null;
    const body = (await res.json()) as GeoapifyResponse;
    return (body.results ?? []).map(fromGeoapify);
  } catch {
    return null;
  }
}

async function searchPhoton(q: string, country: string): Promise<PhotonProperties[]> {
  const url = new URL("https://photon.komoot.io/api/");
  url.searchParams.set("q", q);
  url.searchParams.set("limit", "8");
  url.searchParams.set("lang", "en");
  // Restrict at the source, not only in the filter below. Photon ranks the
  // whole world and returns its top `limit`, so filtering afterwards turns
  // "5 Tagu Place" into eight Philippine streets and then into nothing.
  if (country) url.searchParams.set("countrycode", country);
  try {
    const res = await fetch(url, {
      headers: { "User-Agent": USER_AGENT },
      cache: "no-store",
      // Public Photon takes 3–5 s per query (measured 2026-10-13, from the
      // cluster and from Sydney alike). At 4 s most lookups were aborted
      // and the shopper saw no suggestions at all.
      signal: AbortSignal.timeout(8000),
    });
    if (!res.ok) return [];
    const body = (await res.json()) as PhotonResponse;
    return (body.features ?? []).map((f) => f.properties);
  } catch {
    return [];
  }
}

// A provider answers a street ("Tagu Place") when the house itself is not
// in OSM, and copying that over "5 Tagu pl" threw the number away. Keep the
// number the shopper typed: "5", "12A", "3/15".
const TYPED_NUMBER = /^(\d+[a-z]?(?:\/\d+[a-z]?)?)\s+\S/i;

// `city` is the metro area in Australia and New Zealand ("Sydney" for Kings
// Park), and carriers there validate the suburb, which is `district` here.
// Elsewhere `district` is a ward ("H/W Ward" in Mumbai) and `city` is what
// a label needs.
const SUBURB_COUNTRIES = new Set(["AU", "NZ"]);

function buildCity(p: PhotonProperties): string {
  const city = (p.city ?? "").trim();
  const district = (p.district ?? "").trim();
  if (SUBURB_COUNTRIES.has((p.countrycode ?? "").toUpperCase())) {
    return district || city;
  }
  return city || district;
}

function typedNumberFor(street: string, q: string): string {
  const m = q.trim().match(TYPED_NUMBER);
  if (!m) return "";
  // Only when this suggestion IS the street they are typing — "5" belongs
  // on "Tagu Place" for "5 Tagu pl", not on "Platform 8".
  const word = q.trim().slice(m[1]!.length).trim().split(/\s+/)[0]!.toLowerCase();
  return street.toLowerCase().startsWith(word) ? m[1]! : "";
}

function buildLine1(p: PhotonProperties, q: string): string {
  const street = (p.street ?? p.name ?? "").trim();
  const number = (p.housenumber ?? "").trim() || typedNumberFor(street, q);
  if (number && street) return `${number} ${street}`;
  return street || number;
}

function buildDescription(s: AddressSuggestion, p: PhotonProperties): string {
  const parts = [s.line1, s.city, s.region, s.postal, p.country]
    .map((x) => (x ?? "").trim())
    .filter(Boolean);
  return parts.join(", ");
}

export async function GET(req: Request) {
  const url = new URL(req.url);
  const q = url.searchParams.get("q")?.trim() ?? "";
  const countryParam = (url.searchParams.get("country") ?? "").trim().toUpperCase();
  const country = /^[A-Z]{2}$/.test(countryParam) ? countryParam : "";

  if (q.length < 3) {
    return NextResponse.json({ data: [] });
  }

  const apiKey = process.env.GEOAPIFY_API_KEY?.trim();
  const places =
    (apiKey ? await searchGeoapify(q, country, apiKey) : null) ??
    (await searchPhoton(q, country));

  const seen = new Set<string>();
  const data: AddressSuggestion[] = places
    .filter((p) => !country || (p.countrycode ?? "").toUpperCase() === country)
    .map((p) => {
      const s: AddressSuggestion = {
        description: "",
        line1: buildLine1(p, q),
        city: buildCity(p),
        region: (p.state ?? "").trim(),
        postal: (p.postcode ?? "").trim(),
        country: (p.countrycode ?? "").toUpperCase(),
      };
      s.description = buildDescription(s, p);
      return s;
    })
    // Drop empty rows, and the duplicates OSM holds for one street split
    // into several ways (the screenshot that started this had five).
    .filter((s) => {
      if (s.description.length === 0 || !(s.line1 || s.city)) return false;
      if (seen.has(s.description)) return false;
      seen.add(s.description);
      return true;
    });

  return NextResponse.json({ data });
}

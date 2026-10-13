// GET /api/locations/autocomplete?q=...&country=...
//
// Photon-backed (free OSM) address autocomplete proxy. Mirrors the
// admin route so AddressBook gets the same suggestion popover the
// admin warehouse form has.

import { NextResponse } from "next/server";

export const dynamic = "force-dynamic";

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

interface PhotonFeature {
  properties: PhotonProperties;
}

interface PhotonResponse {
  features: PhotonFeature[];
}

interface AddressSuggestion {
  description: string;
  line1: string;
  city: string;
  region: string;
  postal: string;
  country: string;
}

// Photon answers a street ("Tagu Place") when the house itself is not in
// OSM, and copying that over "5 Tagu pl" threw the number away. Keep the
// number the shopper typed: "5", "12A", "3/15".
const TYPED_NUMBER = /^(\d+[a-z]?(?:\/\d+[a-z]?)?)\s+\S/i;

// Photon's `city` is the metro area in Australia and New Zealand
// ("Sydney" for Kings Park), and carriers there validate the suburb, which
// Photon calls `district`. Elsewhere `district` is a ward ("H/W Ward" in
// Mumbai) and `city` is what a label needs.
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
  const countryFilter = (url.searchParams.get("country") ?? "")
    .trim()
    .toUpperCase();

  if (q.length < 3) {
    return NextResponse.json({ data: [] });
  }

  const photonURL = new URL("https://photon.komoot.io/api/");
  photonURL.searchParams.set("q", q);
  photonURL.searchParams.set("limit", "8");
  photonURL.searchParams.set("lang", "en");
  // Restrict at the source, not only in the filter below. Photon ranks the
  // whole world and returns its top `limit`, so filtering afterwards turns
  // "5 Tagu Place" into eight Philippine streets and then into nothing.
  if (/^[A-Z]{2}$/.test(countryFilter)) {
    photonURL.searchParams.set("countrycode", countryFilter);
  }

  let body: PhotonResponse;
  try {
    const res = await fetch(photonURL.toString(), {
      headers: { "User-Agent": "mark8ly-storefront (+https://mark8ly.com)" },
      cache: "no-store",
      // Public Photon takes 3–5 s per query (measured 2026-10-13, from the
      // cluster and from Sydney alike). At 4 s most lookups were aborted
      // and the shopper saw no suggestions at all.
      signal: AbortSignal.timeout(8000),
    });
    if (!res.ok) return NextResponse.json({ data: [] });
    body = (await res.json()) as PhotonResponse;
  } catch {
    return NextResponse.json({ data: [] });
  }

  const data: AddressSuggestion[] = (body.features ?? [])
    .filter((f) => {
      if (!countryFilter) return true;
      return (f.properties.countrycode ?? "").toUpperCase() === countryFilter;
    })
    .map((f) => {
      const p = f.properties;
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
    .filter((s) => s.description.length > 0 && (s.line1 || s.city));

  return NextResponse.json({ data });
}

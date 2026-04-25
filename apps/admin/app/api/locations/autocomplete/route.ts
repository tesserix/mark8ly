// GET /api/locations/autocomplete?q=...&country=...
//
// Address-suggestion proxy backed by Photon (https://photon.komoot.io)
// — Komoot's free OpenStreetMap geocoder. No API key, no billing
// account, attribution requested in the UI.
//
// Why Photon and not Google Places: Photon is free at unlimited scale
// and good enough for shipping-warehouse address entry where the
// merchant types a partial street address and clicks the right hit.
// Google Places + Maps JavaScript API would be higher-quality but
// requires an API key, billing, and per-session pricing. We can swap
// providers later by changing this single route.
//
// Response shape mirrors what `<AddressFieldset>` consumes — pre-parsed
// AddressValue fragments so the client doesn't have to know about
// Photon / OSM property names.

import { NextResponse } from "next/server";

export const dynamic = "force-dynamic";

interface PhotonProperties {
  name?: string;
  street?: string;
  housenumber?: string;
  postcode?: string;
  city?: string;
  state?: string;
  country?: string;
  countrycode?: string;
  type?: string;
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

function buildLine1(p: PhotonProperties): string {
  const number = (p.housenumber ?? "").trim();
  const street = (p.street ?? p.name ?? "").trim();
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

  let body: PhotonResponse;
  try {
    const res = await fetch(photonURL.toString(), {
      headers: { "User-Agent": "mark8ly-admin (+https://mark8ly.com)" },
      cache: "no-store",
      // Photon usually answers in <300ms; bound it.
      signal: AbortSignal.timeout(4000),
    });
    if (!res.ok) {
      return NextResponse.json({ data: [] });
    }
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
        line1: buildLine1(p),
        city: (p.city ?? "").trim(),
        region: (p.state ?? "").trim(),
        postal: (p.postcode ?? "").trim(),
        country: (p.countrycode ?? "").toUpperCase(),
      };
      s.description = buildDescription(s, p);
      return s;
    })
    // Drop empty rows that Photon occasionally returns for vague queries.
    .filter((s) => s.description.length > 0 && (s.line1 || s.city));

  return NextResponse.json({ data });
}

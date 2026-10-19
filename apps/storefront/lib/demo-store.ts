// Stores Mark8ly runs as public demos. Every page on these shows a "Demo
// store" banner, so a visitor sent from mark8ly.com never mistakes the demo
// for a real shop or its sandbox checkout for a real payment.
//
// Mirrors platform-api's ONBOARDING_DEMO_SLUGS (pkg/config/config.go),
// which classifies these stores as "demo" in the funnel.
// shortcut: duplicated rather than read from the store record, which has no
// demo flag; move to the API response if a second demo store appears.
const DEMO_STORE_SLUGS = new Set(["the-bondi-store"]);

export function isDemoStore(slug: string | null | undefined): boolean {
  return !!slug && DEMO_STORE_SLUGS.has(slug.trim().toLowerCase());
}

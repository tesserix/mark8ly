// Homepage hero copy — single source of truth.
//
// The title tag, OG/Twitter titles and the OG image all read from here,
// so the H1 and what a link preview says can't drift apart again (#599).
//
// Variants to test:
//   B: "Stop taking orders in your DMs." / "Get your own online shop where
//      customers browse, pay and check out — no more chasing payments and
//      addresses."
//   C: "Your own online shop. No cut of your sales." / "Build a store in an
//      afternoon, take card and UPI payments, and keep everything you earn.
//      Free for 90 days."

export const HERO = {
  title: "Create your online shop. Take orders and payments in one place.",
  subtitle:
    "Customers browse and pay on your own store. You manage products and orders from one simple dashboard.",
  terms: "90 days free · No card needed · No platform fee on sales",
};

// The full H1 is 63 characters and Google cuts titles at about 60, so the
// <title> carries the same message, shortened, with the brand.
export const HOME_TITLE = "Create your online shop and take payments · Mark8ly";

// 143 characters, under the ~155 Google renders.
export const HOME_DESCRIPTION =
  "Customers browse and pay on your own store; you manage products and " +
  "orders from one dashboard. 90 days free, no card, no platform fee on sales.";

// TODO(demo-store): set to the public demo store URL. The hero button stays
// hidden while this is empty, so it never links nowhere.
export const DEMO_STORE_URL = "";

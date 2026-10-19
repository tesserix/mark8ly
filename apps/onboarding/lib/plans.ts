// Marketing plan features — single source of truth for every public page
// that lists what a plan includes: the homepage Pricing section and
// /integrations. Prices are not here; they come from
// @repo/ui/subscription (generated from the console catalog).
//
// Every bullet below must be something a merchant on that plan can actually
// get today. Before adding or restoring one:
//   1. Find its `plangate.Feature` constant in
//      services/marketplace-api/internal/plangate/matrix.go, confirm it is
//      enabled (non-Disabled) for that plan, AND grep for the constant
//      outside matrix.go and its tests to confirm a real handler enforces
//      it — a constant that only appears in matrix.go is a hedge for
//      something unbuilt, not a shipped feature (see
//      FeatureCustomCodeInjection).
//   2. Check migrations/ for a later migration that withdrew the feature
//      while its implementation code stayed behind (see
//      000121_retire_untested_paypal — a complete gateway offered to
//      nobody).
//   3. For a numeric limit (stores, emails, images, retention), confirm the
//      number is enforced in code, not just asserted in this file.
// An implementation file existing is NOT evidence a feature is offered.
// This exact mistake shipped twice on the live page — see #544 — before
// anyone checked past "the code exists somewhere."
//
// Tiers are cumulative: each plan lists only what it adds to the one
// before, and Starter leads with what a first-time seller looks for.
// Developer features sit last, on one line.

import type { PlanId } from "@repo/ui/subscription";

export interface PlanMeta {
  id: PlanId;
  name: string;
  tagline: string;
  /** Plan this one builds on; rendered as "Everything in X, plus". */
  includes?: string;
  features: string[];
}

/**
 * API access per plan. plangate grants read scopes on every plan, trial
 * included (FeatureReadAPI, #585), and write on Pro only (FeatureFullAPI).
 */
export const API_ACCESS =
  "Read-only API access on every plan; full read/write access on Pro";

export const PLAN_META: readonly PlanMeta[] = [
  {
    id: "starter",
    name: "Starter",
    tagline: "For your first shop.",
    features: [
      "Unlimited products & orders",
      "Your own domain",
      "Card and local payments at checkout",
      "15,000 campaign emails / mo",
      "Up to 2 stores",
      "Full colour palette & announcement bar",
      "Read-only API and 10 webhook endpoints",
    ],
  },
  {
    id: "studio",
    name: "Studio",
    tagline: "For sellers finding their rhythm.",
    includes: "Starter",
    features: [
      "Up to 5 stores",
      "50 images per product",
      "50,000 campaign emails / mo",
      "Custom CSS & fonts",
      "12-month audit log",
      "25 webhook endpoints",
    ],
  },
  {
    id: "pro",
    name: "Pro",
    tagline: "For larger brands and teams.",
    includes: "Studio",
    features: [
      "Up to 10 stores",
      "Unlimited images",
      "Priority support (4h response)",
      "Forever audit retention",
      // SAML is not implemented — both SSO routes answer 501 for it and
      // there is no SP (mark8ly#820). OIDC is real and configurable at
      // Settings -> Single sign-on.
      "For developers: full read/write API, 100 webhook endpoints, SSO (OpenID Connect)",
    ],
  },
];

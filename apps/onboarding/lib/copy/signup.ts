// Onboarding signup copy — single source of truth.
//
// Editorial voice: calm, precise, never urgent. Every string here is
// visible to a merchant on their first impression of Mark8ly. Keep
// them plain, scannable, and free of exclamation marks.

export const signupCopy = {
  taxIdLabel: "Tax ID (optional)",

  // ─── Promo code (#620) ──────────────────────────────────────────────
  //
  // The field is optional and stays quiet until it has something true to
  // say. Every message below states only what the server actually
  // returned: the number of days comes from the promo definition, never
  // from copy, so a code's terms can change in the console without a
  // deploy here.
  promoLabel: "Promo code (optional)",
  promoPlaceholder: "If you have one",
  promoChecking: "Checking\u2026",
  /** The offer, stated in the code's own numbers. */
  promoAccepted: (days: number) =>
    days === 1
      ? "Adds a day to your free trial."
      : `Adds ${days} days to your free trial.`,
  /** Accepted, but grants nothing we can describe — say nothing rather
   *  than invent a benefit. The code is still carried and still redeemed. */
  promoAcceptedNoTerms: "That code is valid.",
  promoInvalid:
    "We don\u2019t recognise that code, or it has passed its end date. You can carry on without it.",
  /** The code works, just not here. Saying "invalid" would send a merchant
   *  holding a good code away from the page that would have taken it. */
  promoRedeemInBilling:
    "That code applies to a paid plan. Finish signing up and redeem it in Billing.",
  promoMaxRedemptions:
    "That offer has been fully claimed. Your code was genuine \u2014 it has simply run out.",
  promoAlreadyUsed:
    "That code has already been used with this email address.",
  /** We could not ASK. Never rendered as "invalid": a merchant holding a
   *  good code must not be told it is bad because a service was down. */
  promoCheckFailed:
    "We couldn\u2019t check that code just now. You can carry on \u2014 we\u2019ll try again when you finish.",

  // Country-aware help text shown below the tax ID field.
  // Keyed by ISO 3166-1 alpha-2 country code.
  taxIdHelpByCountry: {
    GB: "UK VAT Registration Number (VRN) — 9 or 12 digits, often prefixed GB.",
    DE: "Umsatzsteuer-Identifikationsnummer (USt-IdNr.) — DE followed by 9 digits.",
    FR: "Numéro de TVA intracommunautaire — FR followed by 2 characters and 9 digits.",
    IT: "Partita IVA — 11-digit numeric code.",
    ES: "Número de identificación fiscal (NIF) — letter + 7 digits + letter.",
    NL: "BTW-nummer — NL followed by 9 digits and B01–B99.",
    AU: "Australian Business Number (ABN) — 11 digits.",
    NZ: "New Zealand Business Number (NZBN) — 13 digits.",
    SG: "Unique Entity Number (UEN) — 9 or 10 alphanumeric characters.",
    IN: "Goods and Services Tax Identification Number (GSTIN) — 15 alphanumeric characters.",
    US: "Employer Identification Number (EIN) — 9 digits in XX-XXXXXXX format.",
    CA: "Business Number (BN) — 9 digits, optionally followed by a program account.",
    JP: "Corporate Number (法人番号) — 13 digits.",
    KR: "Business Registration Number (사업자등록번호) — 10 digits.",
    BR: "CNPJ — 14 digits.",
    MX: "Registro Federal de Contribuyentes (RFC) — 12 or 13 alphanumeric characters.",
    ZA: "VAT registration number — 10 digits starting with 4.",
  } as Record<string, string>,

  taxIdHelpFallback:
    "Your business tax or VAT registration number, if applicable.",

  migrationHeading: "Are you migrating an existing store?",
  migrationNewOption: "This is a new store",
  migrationMigratingOption: "I'm migrating from an existing store",
  migrationWhoisLabel: "Current store URL",
  migrationWhoisHelp:
    "We'll verify ownership by looking up WHOIS records for this domain.",
  migrationScreenshotLabel: "Or upload a screenshot of your current storefront",
  migrationScreenshotHelp:
    "A screenshot of your logged-in store admin panel is enough.",
  migrationRequiredError:
    "Provide either a store URL or a screenshot to confirm migration.",
} as const;

export type TaxIdHelpCountry = keyof typeof signupCopy.taxIdHelpByCountry;

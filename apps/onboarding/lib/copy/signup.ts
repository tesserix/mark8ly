// Onboarding signup copy — single source of truth.
//
// Editorial voice: calm, precise, never urgent. Every string here is
// visible to a merchant on their first impression of Mark8ly. Keep
// them plain, scannable, and free of exclamation marks.

export const signupCopy = {
  taxIdLabel: "Tax ID (optional)",

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

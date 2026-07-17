import { addressLines } from "@/lib/customer-address";
import type { CustomerAddress } from "@repo/mobile-shared/api/schemas/customers";

// tsconfig scopes `types` to ["jest"], so declare the Node globals this file's
// fs.readFileSync guard needs rather than widening the project tsconfig.
declare const __dirname: string;

const source = require("fs").readFileSync(
  require("path").join(__dirname, "../app/(tabs)/customers/[id].tsx"),
  "utf8",
);

const FULL: CustomerAddress = {
  id: "a1",
  label: "Home",
  is_default: true,
  name: "Ada Lovelace",
  line1: "12 Analytical Ave",
  line2: "Unit 4",
  city: "Sydney",
  region: "NSW",
  postal_code: "2000",
  country_code: "AU",
};

describe("addressLines", () => {
  it("composes line1, line2, 'city, region postal', country — in order", () => {
    expect(addressLines(FULL)).toEqual([
      "12 Analytical Ave",
      "Unit 4",
      "Sydney, NSW 2000",
      "AU",
    ]);
  });

  it("drops blank optionals instead of leaving empty lines", () => {
    const sparse: CustomerAddress = {
      id: "a2",
      is_default: false,
      name: "No Frills",
      line1: "1 Main St",
      city: "Perth",
      country_code: "AU",
    };
    // No line2, no region/postal — the city line is just the city, no stray commas.
    expect(addressLines(sparse)).toEqual(["1 Main St", "Perth", "AU"]);
  });

  it("joins region and postal with a space, city with a comma", () => {
    const noPostal: CustomerAddress = { ...FULL, postal_code: undefined };
    expect(addressLines(noPostal)).toContain("Sydney, NSW");
  });
});

describe("customer detail — new sections are wired", () => {
  it("renders the Addresses section from customer.addresses", () => {
    expect(source).toContain("customer.addresses");
    expect(source).toContain("<AddressItem");
    expect(source).toContain('label="Addresses"');
  });

  it("surfaces last-order, marketing opt-in, and block reason (all schema-parsed)", () => {
    expect(source).toContain("last_order_at");
    expect(source).toContain("marketing_opt_in");
    expect(source).toContain("customer.block_reason");
  });

  it("collects a typed block reason via the sheet — never the old hardcoded string", () => {
    expect(source).toContain("<BlockReasonSheet");
    expect(source).toContain("blockSheetRef.current?.present()");
    // The hardcoded reason the sheet replaces must be gone.
    expect(source).not.toContain("Blocked from mobile admin");
  });
});

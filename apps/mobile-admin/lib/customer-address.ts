import type { CustomerAddress } from "@repo/mobile-shared/api/schemas/customers";

/**
 * The printable lines of a postal address, blanks dropped. Only line1/city/
 * country_code are guaranteed by the wire (customers_dto.go:47-59); the rest
 * are omitempty, so a naive join would leave stray commas and empty rows.
 */
export function addressLines(address: CustomerAddress): string[] {
  const cityLine = [address.city, [address.region, address.postal_code].filter(Boolean).join(" ")]
    .filter(Boolean)
    .join(", ");
  return [address.line1, address.line2, cityLine, address.country_code].filter(
    (line): line is string => Boolean(line),
  );
}

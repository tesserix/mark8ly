/**
 * Carriers reject a rate request whose ORIGIN address carries no phone.
 * ShipEngine answers 400 `business_rules: "'phone' should not be empty"`,
 * and because the field is serialised `omitempty` an empty string is
 * dropped from the payload entirely rather than sent as blank.
 *
 * The storefront cannot explain any of that to a shopper — it can only
 * report that no rates came back. So the check belongs here, at the point
 * the merchant saves a config that could never quote.
 */
export interface WarehouseAddressLike {
  line1: string;
  city: string;
  postal: string;
  phone: string;
}

/**
 * Returns an error message when the warehouse address cannot produce
 * rates, or null when it is fine to save.
 *
 * A completely empty address is allowed through: a merchant may save
 * credentials first and fill the address later, and blocking that would
 * be a worse trap than the one this prevents.
 */
export function validateWarehouseAddress(
  address: WarehouseAddressLike,
): string | null {
  const hasAddress = Boolean(address.line1 || address.city || address.postal);
  if (!hasAddress) return null;
  if (!address.phone.trim()) {
    return "Add a warehouse phone number — carriers reject rate requests without one, and your storefront would show no delivery options.";
  }
  return null;
}

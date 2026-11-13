// The store's timezone follows the country the merchant picks, not only
// the browser: a laptop left on another zone, or a VPN, should not put an
// Indian store on Chicago time. The browser zone still wins inside a
// country that spans several zones (a Perth seller stays on Perth).
//
// One entry per signup country; the first zone is the country's default.
// shortcut: hand-kept list, extend it when a country is added to the
// signup list (seed/locations.json in platform-api).
const COUNTRY_ZONES: Record<string, readonly string[]> = {
  US: ["America/New_York", "America/Chicago", "America/Denver", "America/Phoenix", "America/Los_Angeles", "America/Anchorage", "Pacific/Honolulu"],
  CA: ["America/Toronto", "America/Vancouver", "America/Edmonton", "America/Winnipeg", "America/Regina", "America/Halifax", "America/St_Johns"],
  BR: ["America/Sao_Paulo", "America/Manaus", "America/Fortaleza", "America/Recife", "America/Belem", "America/Bahia", "America/Cuiaba", "America/Porto_Velho", "America/Rio_Branco", "America/Noronha"],
  GB: ["Europe/London"],
  DE: ["Europe/Berlin"],
  FR: ["Europe/Paris"],
  IT: ["Europe/Rome"],
  ES: ["Europe/Madrid", "Atlantic/Canary"],
  NL: ["Europe/Amsterdam"],
  IN: ["Asia/Kolkata", "Asia/Calcutta"],
  SG: ["Asia/Singapore"],
  MY: ["Asia/Kuala_Lumpur", "Asia/Kuching"],
  TH: ["Asia/Bangkok"],
  PH: ["Asia/Manila"],
  ID: ["Asia/Jakarta", "Asia/Pontianak", "Asia/Makassar", "Asia/Jayapura"],
  JP: ["Asia/Tokyo"],
  AU: ["Australia/Sydney", "Australia/Melbourne", "Australia/Brisbane", "Australia/Adelaide", "Australia/Perth", "Australia/Hobart", "Australia/Darwin"],
  NZ: ["Pacific/Auckland"],
  AE: ["Asia/Dubai"],
  SA: ["Asia/Riyadh"],
};

/** The IANA zone a new store should run on. */
export function storeTimezone(countryCode: string, browserZone: string): string {
  const zones = COUNTRY_ZONES[countryCode];
  if (zones) return zones.includes(browserZone) ? browserZone : zones[0]!;
  return browserZone || "UTC";
}

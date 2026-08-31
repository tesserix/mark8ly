/**
 * Pickup automation is a Delhivery capability: DelhiveryCarrier is the
 * only carrier implementing SchedulePickup (marketplace-api
 * shipping/delhivery.go:941). The backend simply no-ops for everyone
 * else, so the form used to show the controls to every provider and
 * default them ON — an AU store configuring ShipEngine was greeted by a
 * pre-ticked "Auto-schedule Delhivery pickup", which reads either as a
 * broken store or as a carrier they never chose.
 *
 * Keep this list in step with the carriers implementing SchedulePickup.
 */
const PICKUP_AUTOMATION_PROVIDERS = new Set(["delhivery"]);

export function supportsPickupAutomation(provider: string): boolean {
  return PICKUP_AUTOMATION_PROVIDERS.has(provider.trim().toLowerCase());
}

/**
 * defaultAutoSchedulePickup decides the checkbox's initial state.
 * A saved value always wins, so a merchant who deliberately turned it
 * off keeps it off. Only the *unset* default is provider-aware.
 */
export function defaultAutoSchedulePickup(
  provider: string,
  saved: boolean | undefined,
): boolean {
  if (saved !== undefined) return saved;
  return supportsPickupAutomation(provider);
}

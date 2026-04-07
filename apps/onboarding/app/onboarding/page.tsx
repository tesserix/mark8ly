import { locations } from "@/lib/api/platform-api";
import { OnboardingWizard } from "@/components/onboarding/OnboardingWizard";

// Server component: fetches reference data once on the server and hands it
// down to the client wizard. The wizard owns the step state from there.
//
// We fetch countries, currencies, and timezones server-side so the
// dropdowns are populated instantly with no flash. Caches via Next's
// default fetch caching (1 hour).
export default async function OnboardingPage() {
  const [countries, currencies, timezones] = await Promise.all([
    locations.listCountries(),
    locations.listCurrencies(),
    locations.listTimezones(),
  ]);

  return (
    <main className="min-h-screen flex items-center justify-center px-4 py-12">
      <OnboardingWizard
        countries={countries}
        currencies={currencies}
        timezones={timezones}
      />
    </main>
  );
}

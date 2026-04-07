import { locations } from "@/lib/api/platform-api";
import { OnboardingForm } from "@/components/onboarding/OnboardingForm";

// Server component: fetches reference data once on the server and hands
// it down to the single-page form.
export default async function OnboardingPage() {
  const [countries, currencies, timezones] = await Promise.all([
    locations.listCountries(),
    locations.listCurrencies(),
    locations.listTimezones(),
  ]);

  return (
    <main className="min-h-screen flex items-center justify-center px-4 py-12">
      <OnboardingForm
        countries={countries}
        currencies={currencies}
        timezones={timezones}
      />
    </main>
  );
}

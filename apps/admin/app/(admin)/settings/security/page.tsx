import type { Metadata } from "next";
import { cookies } from "next/headers";
import { AdminPage, PageSection } from "@/components/layout";
import { getMyProviders } from "@/lib/auth/auth-bff";
import { SecurityClient } from "./SecurityClient";

export const metadata: Metadata = { title: "Security" };

export default async function SecurityPage() {
  const c = await cookies();
  const cookieHeader = c
    .getAll()
    .map((x) => `${x.name}=${x.value}`)
    .join("; ");

  let providers: { providerId: string; email?: string }[] = [];
  let loadError: string | null = null;
  try {
    const result = await getMyProviders(cookieHeader);
    providers = result.providers;
  } catch (err) {
    loadError =
      err instanceof Error ? err.message : "Could not load providers.";
  }

  return (
    <AdminPage
      eyebrow="Settings"
      title="Security"
      description="Manage how you sign in to your store dashboard."
    >
      <PageSection
        title="Linked sign-in methods"
        description="Add Google to sign in faster, or remove a method you no longer use (contact support to remove a sign-in method for now)."
      >
        <SecurityClient initialProviders={providers} loadError={loadError} />
      </PageSection>
    </AdminPage>
  );
}

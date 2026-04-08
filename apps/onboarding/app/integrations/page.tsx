import { MarketingStub } from "@/components/marketing/primitives";

export const metadata = {
  title: "Integrations",
};

export default function IntegrationsPage() {
  return (
    <MarketingStub
      eyebrow="Integrations"
      title={<>A small, considered set.</>}
      body="We&rsquo;re picking integrations the way an editor picks a list of recommended tools — only the ones we&rsquo;d use ourselves. Payment processors, shipping carriers, analytics, and email tools are all in progress. The full list will appear here as each one lands."
      primaryCta={{ href: "/onboarding", label: "Open your store" }}
      secondaryCta={{ href: "/contact", label: "Suggest one" }}
    />
  );
}

import { MarketingStub } from "@/components/marketing/primitives";

export const metadata = {
  title: "Security & compliance",
};

export default function LegalPage() {
  return (
    <MarketingStub
      eyebrow="Security & compliance"
      title={<>Treated with care.</>}
      body="TLS everywhere, PCI-compliant payment routing, GDPR-aligned data handling, and regular third-party reviews. We&rsquo;re writing up the full security posture — for now, email security@mark8ly.com for disclosures or compliance questions."
      primaryCta={{
        href: "mailto:security@mark8ly.com",
        label: "Email security",
      }}
      secondaryCta={{ href: "/privacy", label: "Read the privacy policy" }}
    />
  );
}

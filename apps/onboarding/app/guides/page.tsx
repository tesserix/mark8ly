import { MarketingStub } from "@/components/marketing/primitives";

export const metadata = {
  title: "Guides",
  robots: { index: false, follow: true },
};

export default function GuidesPage() {
  return (
    <MarketingStub
      eyebrow="Guides"
      title={<>Walkthroughs, coming soon.</>}
      body="We&rsquo;re writing a small set of focused guides for new merchants: first product, first sale, first domain, first tax return. Each one will be short, honest, and free of filler. Back here when they ship."
      primaryCta={{ href: "/onboarding", label: "Open your store" }}
      secondaryCta={{ href: "/help", label: "Contact support" }}
    />
  );
}

import { MarketingStub } from "@/components/marketing/primitives";

export const metadata = {
  title: "Help",
};

export default function HelpPage() {
  return (
    <MarketingStub
      eyebrow="Help"
      title={<>Answers are on the way.</>}
      body="Our full help centre is being written alongside the product. In the meantime, the best way to get help is to email us directly — a real person reads every message."
      primaryCta={{ href: "/onboarding", label: "Open your store" }}
      secondaryCta={{ href: "/contact", label: "Email us" }}
    />
  );
}

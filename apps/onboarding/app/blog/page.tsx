import { MarketingStub } from "@/components/marketing/primitives";

export const metadata = {
  title: "Journal",
};

export default function BlogPage() {
  return (
    <MarketingStub
      eyebrow="Journal"
      title={<>Writing, coming soon.</>}
      body="We&rsquo;re preparing a small, quiet journal about what we&rsquo;re building and the merchants we&rsquo;re building it for. No content for content&rsquo;s sake — only things worth your time. Check back, or email us to be notified when the first piece goes up."
      primaryCta={{ href: "/onboarding", label: "Open your store" }}
      secondaryCta={{ href: "/contact", label: "Get in touch" }}
    />
  );
}

import { MarketingStub } from "@/components/marketing/primitives";
import { JournalSignupForm } from "@/components/marketing/JournalSignupForm";

export const metadata = {
  title: "Journal",
  robots: { index: false, follow: true },
};

export default function BlogPage() {
  return (
    <MarketingStub
      eyebrow="Journal"
      title={<>Writing, coming soon.</>}
      body="We&rsquo;re preparing a small, quiet journal about what we&rsquo;re building and the merchants we&rsquo;re building it for. No content for content&rsquo;s sake — only things worth your time. Check back, or leave your email below to hear when the first piece goes up."
      primaryCta={{ href: "/onboarding", label: "Open your store" }}
      secondaryCta={{ href: "/contact", label: "Get in touch" }}
    >
      <JournalSignupForm />
    </MarketingStub>
  );
}

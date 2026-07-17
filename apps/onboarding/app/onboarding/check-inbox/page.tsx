import { CheckInbox } from "@/components/onboarding/CheckInbox";
import { PostSubmitShell } from "@/components/onboarding/PostSubmitShell";

// Funnel intermediate — never index.
export const metadata = {
  title: "Check your inbox",
  robots: { index: false, follow: false },
};

export default function CheckInboxPage() {
  return (
    <PostSubmitShell
      step={2}
      eyebrow="Email confirmation"
      title="Your store is one click away."
      description="We sent your verification link. Open it on this device and we’ll carry you straight into the last setup step."
    >
      <CheckInbox />
    </PostSubmitShell>
  );
}

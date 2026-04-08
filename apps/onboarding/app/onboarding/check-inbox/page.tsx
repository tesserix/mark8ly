import { CheckInbox } from "@/components/onboarding/CheckInbox";
import { PostSubmitShell } from "@/components/onboarding/PostSubmitShell";

export default function CheckInboxPage() {
  return (
    <PostSubmitShell
      eyebrow="Email confirmation"
      title="Your store is one click away."
      description="We sent your verification link. Open it on this device and we’ll carry you straight into the last setup step."
    >
      <CheckInbox />
    </PostSubmitShell>
  );
}

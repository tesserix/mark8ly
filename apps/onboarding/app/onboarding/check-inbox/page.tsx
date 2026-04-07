import { CheckInbox } from "@/components/onboarding/CheckInbox";
import { PostSubmitShell } from "@/components/onboarding/PostSubmitShell";

export default function CheckInboxPage() {
  return (
    <PostSubmitShell
      eyebrow="Email confirmation"
      title="Your store is one click away."
      description="We’ve sent the verification link. Keep the flow feeling calm and premium while the customer completes the final step."
    >
      <CheckInbox />
    </PostSubmitShell>
  );
}

import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import { TestimonialForm } from "@/components/settings/TestimonialForm";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getTestimonial } from "@/lib/api/testimonials";

export default async function TestimonialPage() {
  const { role, currentStore, userId } = await getServerSessionContext();
  const editable = canEditSettings(role);
  let initial = null;
  let failed = false;
  if (editable && currentStore) {
    try {
      initial = await getTestimonial(currentStore.id, userId);
    } catch {
      failed = true;
    }
  }
  return (
    <AdminPage
      eyebrow="Account"
      title="Your testimonial"
      description="Tell us about your experience with Mark8ly."
      readOnlyNotice={
        !editable && role ? <ReadOnlyNotice role={role} /> : undefined
      }
    >
      {!currentStore ? (
        <p>Select a store first.</p>
      ) : failed ? (
        <p role="alert">
          Could not load your testimonial. Reload the page to try again.
        </p>
      ) : editable ? (
        <TestimonialForm
          key={currentStore.id}
          initial={initial}
          storeName={currentStore.name}
        />
      ) : null}
    </AdminPage>
  );
}

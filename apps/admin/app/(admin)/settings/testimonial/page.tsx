import { AdminPage } from "@/components/layout";
import { TestimonialForm } from "@/components/settings/TestimonialForm";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getTestimonial } from "@/lib/api/testimonials";

export default async function TestimonialPage() {
  const { role, currentStore, userId } = await getServerSessionContext();
  // One review per store, written in the owner's voice; admins would overwrite it.
  const editable = role === "owner";
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
        !editable ? (
          <p className="text-sm text-warning">
            Only the store owner can write the testimonial.
          </p>
        ) : undefined
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

import { MailLink } from "@repo/ui/mail-link";
import { MarketingPage, PageHero, Prose } from "@/components/marketing/primitives";

export const metadata = {
  title: "Delete your account",
  description:
    "How to permanently delete your Mark8ly account and data. Self-service from the mobile app or admin dashboard, what is removed, what is retained under Australian law, and how to request deletion if you can't sign in.",
  alternates: { canonical: "/delete-account" },
};

export default function DeleteAccountPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Account &amp; data"
        title={<>Delete your account.</>}
        lede="You can permanently delete your Mark8ly account and its data at any time. Here&rsquo;s exactly how it works, what&rsquo;s removed, and what we&rsquo;re required to keep."
      />

      <Prose>
        <h2>1. Delete it yourself</h2>
        <p>
          The fastest way to delete your account is from a signed-in session.
          Either path removes your account permanently &mdash; there is no
          separate approval step.
        </p>
        <ul>
          <li>
            <strong>Mobile app (Mark8ly Admin):</strong> open{" "}
            <strong>More &rarr; Account</strong>, scroll to{" "}
            <strong>Delete account</strong>, type <strong>DELETE</strong> to
            confirm and tap <strong>Delete account</strong>. You&rsquo;ll be
            signed out immediately.
          </li>
          <li>
            <strong>Admin dashboard on the web:</strong> go to{" "}
            <strong>Settings &rarr; Account</strong> and find the{" "}
            <strong>Delete account</strong> section (store owners see{" "}
            <strong>Delete store</strong>; staff see{" "}
            <strong>Remove my access</strong>), then type <strong>DELETE</strong>{" "}
            to confirm. You&rsquo;ll be signed out immediately.
          </li>
        </ul>

        <h2>2. What gets removed</h2>
        <p>
          What we delete depends on your role, because a store can have more
          than one person on it.
        </p>
        <ul>
          <li>
            <strong>If you own the store:</strong> deleting your account tears
            down the entire store &mdash; products, orders, customers,
            categories, staff, vendors, discounts and all other store data
            &mdash; along with your sign-in credentials and access permissions.
            This cannot be undone.
          </li>
          <li>
            <strong>If you&rsquo;re a staff member:</strong> we remove your
            access, your sign-in credentials and your permissions. The store
            itself keeps running for its owner and other staff.
          </li>
        </ul>
        <p>
          Your login is deleted from our identity provider and your permission
          records are removed at the same time.
        </p>

        <h2>3. What we&rsquo;re required to keep</h2>
        <p>
          A small amount of data is retained after deletion where the law
          requires it. This mirrors section 7 of our{" "}
          <a href="/privacy">privacy policy</a>.
        </p>
        <ul>
          <li>
            <strong>Billing and tax records:</strong> retained for seven years
            to meet Australian tax and corporate-record obligations, then
            deleted. These are kept as legal records and are not used to run an
            account.
          </li>
          <li>
            <strong>Support correspondence:</strong> retained for two years.
          </li>
          <li>
            <strong>Backups:</strong> rotated on a schedule not exceeding 90
            days; your deletion propagates to backups on the next rotation.
          </li>
        </ul>

        <h2>4. Timeline</h2>
        <p>
          Access is revoked the moment you confirm. Your store and account data
          are purged from our live systems right away, and removed from backups
          on the next rotation cycle (within 90 days). Records we&rsquo;re
          legally required to keep are held for the periods above and then
          deleted.
        </p>

        <h2>5. Can&rsquo;t sign in?</h2>
        <p>
          If you&rsquo;ve lost access to your account and can&rsquo;t use either
          self-service option above, email{" "}
          <MailLink email="privacy@mark8ly.com" /> from the
          address on your account and ask us to delete it. We&rsquo;ll verify
          you own the account and complete the deletion for you.
        </p>
      </Prose>
    </MarketingPage>
  );
}

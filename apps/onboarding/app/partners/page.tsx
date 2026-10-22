import { MailLink } from "@repo/ui/mail-link";
import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

// Reached from partner outreach only while the programme signs its first
// agencies; not in the sitemap or footer, and kept out of search until then.
export const metadata = {
  title: "Agency partners",
  description:
    "The Mark8ly Agency Partner Programme: 20% of the subscription fees we collect from every store you refer, for as long as it pays.",
  alternates: { canonical: "/partners" },
  robots: { index: false, follow: false },
};

export default function PartnersPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Agency Partner Programme · October 2026"
        title={<>Partner with Mark8ly.</>}
        lede="For studios and freelancers who build brands for independent sellers. Set your clients up with their own store, keep billing for your work, and earn a share of every subscription you bring in."
      />

      <Prose>
        <h2>What you get</h2>
        <ul>
          <li>
            <strong>20% of the subscription fees we collect</strong> from each
            store you refer, for as long as that store keeps paying for
            Mark8ly.
          </li>
          <li>
            <strong>Your own partner code and link</strong>, for example{" "}
            <code>mark8ly.com/onboarding?promo=YOURCODE</code>. Clients who
            sign up with it get 30 extra days of free trial on top of the
            standard 90.
          </li>
          <li>
            <strong>No fee to join</strong> and no targets to hit.
          </li>
        </ul>
        <p>
          To join, email <MailLink email="partners@mark8ly.com" /> with your
          studio&rsquo;s name and website, and we&rsquo;ll send your code.
        </p>

        <h2>Programme terms</h2>
        <p>
          The programme is run by <strong>Tesserix Pty Ltd</strong> (ABN 59
          694 070 865) of New South Wales, Australia (&ldquo;we&rdquo;). These
          terms apply to an agency, studio or freelancer that joins it
          (&ldquo;you&rdquo;).
        </p>

        <h3>What counts as a referred store</h3>
        <ul>
          <li>
            The store signed up with your code or your link, <strong>or</strong>{" "}
            you tell us about it within 30 days of its signup.
          </li>
          <li>
            One partner per store. If two partners claim the same store, the
            code used at signup wins.
          </li>
          <li>
            Your own stores, and stores of your own business, don&rsquo;t earn
            commission.
          </li>
          <li>
            The store owner creates and owns their account. You can build and
            run it for them with their permission.
          </li>
        </ul>

        <h3>How commission is worked out</h3>
        <ul>
          <li>
            20% of the subscription fees we actually receive from the store,
            excluding GST or other sales taxes and payment processing fees.
          </li>
          <li>
            No commission on the free trial, refunds, chargebacks or unpaid
            invoices. If a payment we paid commission on is later refunded,
            we deduct it from your next payout.
          </li>
          <li>
            Commission is on Mark8ly subscription fees only. It doesn&rsquo;t
            apply to the store&rsquo;s own sales.
          </li>
        </ul>

        <h3>How you get paid</h3>
        <ul>
          <li>
            Quarterly by bank transfer, within 30 days of the end of each
            quarter (March, June, September, December).
          </li>
          <li>
            We pay once you&rsquo;re owed at least A$50. Anything less carries
            over to the next quarter.
          </li>
          <li>Each payout comes with a statement listing the stores and amounts.</li>
          <li>
            You&rsquo;re responsible for your own taxes. Australian partners
            registered for GST send us a tax invoice for each payout.
          </li>
        </ul>

        <h3>Your work with clients</h3>
        <ul>
          <li>
            You set your own prices for your services and bill clients
            directly. We don&rsquo;t take a cut of that.
          </li>
          <li>
            We don&rsquo;t build custom features or integrations for partners
            at this stage. Support for the stores comes through our normal
            channels.
          </li>
          <li>
            Describe Mark8ly accurately: no promises we haven&rsquo;t made,
            and say you&rsquo;re a partner when you recommend us.
          </li>
          <li>
            Promote your link in ways that follow spam and privacy laws. No
            unsolicited bulk email, no buying ads on the Mark8ly name, and no
            fake reviews.
          </li>
        </ul>

        <h3>Changes and ending</h3>
        <ul>
          <li>
            <strong>Stores you&rsquo;ve referred keep earning 20% for as long
            as they pay</strong>, even if we later change these terms or close
            the programme to new referrals.
          </li>
          <li>
            We can change other terms, or the rate for future referrals, with
            90 days&rsquo; notice by email.
          </li>
          <li>
            Either side can leave at any time by email. Commission already
            earned is still paid.
          </li>
          <li>
            If a partner breaks these terms (for example spam, fake signups
            or misleading claims), we can end the partnership straight away
            and withhold commission on the affected stores.
          </li>
        </ul>
        <p>
          The stores themselves remain under our{" "}
          <a href="/terms">Terms of service</a>. Questions:{" "}
          <MailLink email="partners@mark8ly.com" />.
        </p>
      </Prose>
    </MarketingPage>
  );
}

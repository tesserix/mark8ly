import { MailLink } from "@repo/ui/mail-link";
import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Refunds",
  description:
    "Mark8ly refund policy. Ninety-day free trial, fourteen-day post-purchase refund window, Australian Consumer Law protections honoured.",
  alternates: { canonical: "/refunds" },
};

export default function RefundsPolicyPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · April 2026"
        title={<>Refunds.</>}
        lede="Ninety days free, fourteen days to change your mind after that. Australian Consumer Law always applies."
      />

      <Prose>
        <p>
          This policy covers what Mark8ly charges merchants for subscriptions
          and add-ons. It does <strong>not</strong> cover refunds a merchant
          owes their own customers — that&rsquo;s up to each store&rsquo;s own
          policy.
        </p>

        <h2>1. The free trial</h2>
        <p>
          Every Mark8ly account starts with a ninety-day free trial. No
          credit card is required to start. You can cancel at any time during
          the trial and you will never be charged.
        </p>

        <h2>2. Fourteen-day refund window</h2>
        <p>
          After your first paid charge, you have fourteen days to request a
          full refund — no questions asked. Email{" "}
          <MailLink email="support@mark8ly.com" /> with
          your store name and the invoice number. Typical turnaround is five
          business days; funds reappear on the original payment method.
        </p>

        <h2>3. After fourteen days</h2>
        <p>
          Outside the fourteen-day window, subscription charges are
          non-refundable. You can cancel any time from your account
          settings, and cancellation takes effect at the end of your current
          billing period. Downgrades also take effect at the end of the
          current period. Upgrades prorate.
        </p>

        <h2>4. Add-ons and one-offs</h2>
        <p>
          The <strong>White-label App</strong> add-on for the Pro plan is
          billed annually and co-terminated with your Pro renewal. Refunds on
          the add-on follow the same fourteen-day rule from the initial
          purchase date. One-off purchases (e.g., premium themes when
          available) are refundable within fourteen days of purchase unless
          they have already been downloaded or activated.
        </p>

        <h2>5. Australian Consumer Law</h2>
        <p>
          Nothing in this policy excludes, restricts, or modifies any
          consumer guarantee, right, or remedy that you have under the
          Australian Consumer Law (Schedule 2 of the Competition and Consumer
          Act 2010 (Cth)). If you are an Australian consumer and Mark8ly
          fails to meet a consumer guarantee, you may be entitled to a
          refund, replacement, or compensation for reasonably foreseeable
          loss or damage under the ACL, in addition to anything this policy
          offers.
        </p>

        <h2>6. Chargebacks</h2>
        <p>
          We&rsquo;d rather fix a problem than fight a chargeback. If you
          think a charge is wrong, email us first — we respond same-day on
          business days. Filing a chargeback without contacting us may lead
          to account review or suspension while the dispute is investigated.
        </p>

        <h2>7. How to request a refund</h2>
        <ol style={{ listStyleType: "decimal", paddingLeft: "1.5rem" }}>
          <li>Email <MailLink email="support@mark8ly.com" /></li>
          <li>Include your store name and the invoice number</li>
          <li>Tell us briefly what happened</li>
        </ol>
        <p>
          We respond within two business days and typically process approved
          refunds within five business days. Funds take an additional 3–10
          business days to reach your bank, depending on your card issuer.
        </p>

        <h2>8. Contact</h2>
        <ul>
          <li>
            <strong>Billing or refund questions:</strong>{" "}
            <MailLink email="support@mark8ly.com" />
          </li>
          <li>
            <strong>Legal:</strong>{" "}
            <MailLink email="legal@mark8ly.com" />
          </li>
        </ul>

        <h2>9. About Tesserix</h2>
        <p>
          Mark8ly is a product of <strong>Tesserix Pty Ltd</strong> (ACN 694
          070 865, ABN 59 694 070 865), registered in New South Wales,
          Australia.
        </p>
      </Prose>
    </MarketingPage>
  );
}

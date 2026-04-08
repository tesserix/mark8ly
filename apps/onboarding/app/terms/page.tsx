import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Terms",
  description:
    "Mark8ly Terms of Service. Six months free, then $9.99 flat per month. No transaction fees from Mark8ly. Cancel any time, export your data, no penalties.",
  alternates: { canonical: "/terms" },
};

export default function TermsOfServicePage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · January 2026"
        title={<>Terms of service.</>}
        lede="The rules of the road, written as plainly as we can make them while still being a real legal agreement."
      />

      <Prose>
        <p>
          Welcome to Mark8ly. These terms govern your use of the platform. By
          creating an account or using the service, you agree to them.
        </p>

        <h2>1. Accepting these terms</h2>
        <p>
          By creating a Mark8ly account or using our services, you agree to
          these terms of service, our privacy policy, and our acceptable use
          policy.
        </p>
        <p>
          <strong>Age requirement:</strong> you must be at least 18 years old
          to use Mark8ly.
        </p>

        <h2>2. The service</h2>
        <h3>What we provide</h3>
        <ul>
          <li>Store hosting and infrastructure</li>
          <li>Product and catalogue management tools</li>
          <li>Payment processing integration</li>
          <li>Order management</li>
          <li>Human customer support</li>
        </ul>
        <h3>What we don&rsquo;t provide</h3>
        <ul>
          <li>
            We don&rsquo;t manufacture, ship, or fulfil your products.
          </li>
          <li>
            We don&rsquo;t provide legal, tax, or business advice.
          </li>
          <li>We don&rsquo;t guarantee sales or revenue.</li>
        </ul>

        <h2>3. Pricing and payments</h2>
        <p>
          <strong>Free trial:</strong> six months completely free. No credit
          card required.
        </p>
        <p>
          <strong>After the free period:</strong> $9.99 a month, flat. No
          extra platform transaction fees from Mark8ly. Cancel any time.
        </p>
        <h3>Refunds</h3>
        <p>
          We offer refunds within 14 days of purchase — email{" "}
          <a href="mailto:support@mark8ly.com">support@mark8ly.com</a>. After
          14 days, charges are non-refundable, but you can cancel any time.
        </p>

        <h2>4. Acceptable use</h2>
        <h3>You may not use Mark8ly to</h3>
        <ul>
          <li>Sell illegal, counterfeit, or restricted items</li>
          <li>
            Engage in fraudulent activity, schemes, or money laundering
          </li>
          <li>Upload malware or attempt unauthorised access</li>
          <li>Interfere with other merchants&rsquo; stores</li>
        </ul>

        <h2>5. Termination</h2>
        <h3>Your right to cancel</h3>
        <ul>
          <li>Cancel any time from your account settings</li>
          <li>No cancellation fees or penalties</li>
          <li>Download your data before cancelling</li>
        </ul>

        <h2>6. Limitation of liability</h2>
        <p>
          To the fullest extent permitted by law, Mark8ly&rsquo;s total
          liability is limited to the amount you paid us in the 12 months
          before the claim arose.
        </p>

        <h2>7. Contact</h2>
        <ul>
          <li>
            <strong>Legal questions:</strong>{" "}
            <a href="mailto:legal@mark8ly.com">legal@mark8ly.com</a>
          </li>
          <li>
            <strong>General support:</strong>{" "}
            <a href="mailto:support@mark8ly.com">support@mark8ly.com</a>
          </li>
        </ul>
      </Prose>
    </MarketingPage>
  );
}

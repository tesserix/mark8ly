import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Terms",
  description:
    "Mark8ly Terms of Service. Ninety-day free trial, then one of three clear plans from $15 a month billed yearly. No transaction fees from Mark8ly. Governed by New South Wales, Australia. Australian Consumer Law always applies.",
  alternates: { canonical: "/terms" },
};

export default function TermsOfServicePage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · April 2026"
        title={<>Terms of service.</>}
        lede="The rules of the road, written as plainly as we can make them while still being a real legal agreement."
      />

      <Prose>
        <p>
          Welcome to Mark8ly. These terms govern your use of the platform. By
          creating an account or using the service, you agree to them.
        </p>

        <h2>0. Parties and definitions</h2>
        <p>
          This agreement is between <strong>Tesserix Pty Ltd</strong> (ACN
          694 070 865, ABN 59 694 070 865) of New South Wales, Australia
          (&ldquo;<strong>Tesserix</strong>&rdquo;, &ldquo;
          <strong>Mark8ly</strong>&rdquo;, &ldquo;we&rdquo;, &ldquo;us&rdquo;,
          &ldquo;our&rdquo;) and the person or entity that creates a Mark8ly
          account (&ldquo;<strong>you</strong>&rdquo;, &ldquo;your&rdquo;).
          &ldquo;<strong>Service</strong>&rdquo; means the Mark8ly ecommerce
          platform at <a href="https://mark8ly.com">mark8ly.com</a> and
          related apps, APIs, and mobile applications.
        </p>

        <h2>1. Accepting these terms</h2>
        <p>
          By creating a Mark8ly account or using the Service, you agree to
          these terms, the <a href="/privacy">Privacy policy</a>, the{" "}
          <a href="/acceptable-use">Acceptable use policy</a>, the{" "}
          <a href="/cookies">Cookie policy</a>, the{" "}
          <a href="/refunds">Refunds policy</a>, and (where you act as a
          data controller) the <a href="/dpa">Data Processing Addendum</a>.
        </p>
        <p>
          <strong>Age requirement:</strong> you must be at least 18 years
          old, or the legal age of majority in your jurisdiction, to use
          Mark8ly.
        </p>
        <p>
          <strong>Business accounts:</strong> if you accept these terms on
          behalf of a company, you warrant that you have authority to do so.
        </p>

        <h2>2. The service</h2>
        <h3>What we provide</h3>
        <ul>
          <li>Store hosting and infrastructure</li>
          <li>Product and catalogue management tools</li>
          <li>
            Payment processing integration (via Stripe, Razorpay, and Cashfree
            Payments)
          </li>
          <li>Order management</li>
          <li>Human customer support</li>
        </ul>
        <h3>What we don&rsquo;t provide</h3>
        <ul>
          <li>We don&rsquo;t manufacture, ship, or fulfil your products.</li>
          <li>We don&rsquo;t provide legal, tax, or business advice.</li>
          <li>We don&rsquo;t guarantee sales or revenue.</li>
          <li>
            We don&rsquo;t act as a party to transactions between you and
            your customers; those are between you and them.
          </li>
        </ul>

        <h2>3. Pricing and payments</h2>
        <p>
          <strong>Free trial:</strong> ninety days completely free. No
          credit card required to start.
        </p>
        <p>
          <strong>After the free period:</strong> three plans — Starter,
          Studio, and Pro — from $19 a month billed monthly, or from $15 a
          month billed yearly. Annual billing available on
          every plan. No extra platform transaction fees from Mark8ly.
          Upgrades prorate; downgrades take effect at the end of your
          current period. Cancel any time.
        </p>
        <p>
          <strong>Pro&nbsp;+&nbsp;White-label App add-on:</strong> Pro-plan
          merchants may purchase the optional White-label App add-on, billed
          annually and co-terminated with the Pro renewal date.
        </p>
        <p>
          <strong>Taxes:</strong> plan prices are stated exclusive of GST
          and other applicable taxes, which we add at invoice where
          required.
        </p>
        <p>
          <strong>Payment methods:</strong> Mark8ly subscriptions are
          charged through Stripe (global) or Razorpay (India). See{" "}
          <a href="/sub-processors">/sub-processors</a>.
        </p>
        <p>
          <strong>Who collects your payment:</strong> Subscription payments are
          collected and settled by <strong>Tesserix Pty Ltd</strong>. That is
          the name that may appear on your bank or card statement, whether you
          are billed through Stripe or Razorpay. If we begin billing you
          through a different group entity, we will tell you before it takes
          effect.
        </p>

        <h2>4. Acceptable use</h2>
        <p>
          You must use the Service in accordance with our{" "}
          <a href="/acceptable-use">Acceptable use policy</a>. In short, you
          may not use Mark8ly to sell illegal, counterfeit, or restricted
          items; engage in fraudulent activity, schemes, or money
          laundering; upload malware or attempt unauthorised access; or
          interfere with other merchants&rsquo; stores. Violations may
          result in suspension or termination.
        </p>

        <h2>5. Your data and content</h2>
        <h3>5.1 Ownership</h3>
        <p>
          You retain all rights in the content you upload to Mark8ly —
          products, images, text, customer data. We do not claim ownership.
        </p>
        <h3>5.2 Licence to us</h3>
        <p>
          To run the Service for you, you grant Tesserix a limited,
          non-exclusive, worldwide, royalty-free licence to host, copy,
          display, transmit, cache, and back up your content, solely as
          needed to provide the Service, maintain security and availability,
          and comply with law. This licence ends when your content is
          deleted, subject to backup-rotation as described in the Privacy
          policy.
        </p>
        <h3>5.3 Data export</h3>
        <p>
          You can export your data at any time through the admin dashboard
          and for at least 30 days after account closure.
        </p>
        <h3>5.4 Data processing</h3>
        <p>
          Where you act as a data controller and we process personal data
          on your behalf, the <a href="/dpa">Data Processing Addendum</a>{" "}
          applies and is auto-accepted on signup.
        </p>

        <h2>6. Intellectual property</h2>
        <p>
          The Service, including its software, design, and underlying
          intellectual property, is owned by Tesserix and its licensors.
          Third-party trademarks and logos remain the property of their
          respective owners.
        </p>

        <h2>7. Copyright takedown (Copyright Act 1968 (Cth))</h2>
        <p>
          If you believe content on a Mark8ly-hosted store infringes your
          copyright, send a notice to{" "}
          <a href="mailto:legal@mark8ly.com">legal@mark8ly.com</a> including:
        </p>
        <ul>
          <li>your contact details</li>
          <li>identification of the copyrighted work</li>
          <li>the URL(s) of the infringing material</li>
          <li>a statement that you have a good-faith belief the use is not authorised</li>
          <li>a statement, under penalty of perjury, that the information in the notice is accurate and that you are the rights-holder or authorised to act on their behalf</li>
          <li>your physical or electronic signature</li>
        </ul>
        <p>
          We act on valid notices promptly. If we remove your content and
          you believe the notice was wrong or in error, you may send a
          counter-notice to the same address. We honour equivalent takedown
          processes under foreign law (e.g., DMCA for US jurisdiction) where
          they apply.
        </p>

        <h2>8. Service levels and changes</h2>
        <p>
          We aim for high availability but <strong>do not offer an uptime
          warranty</strong> on current plans. Planned maintenance windows
          are announced in-app. We may add, change, or remove features at
          any time; material removals affecting your active plan will be
          announced with reasonable notice.
        </p>

        <h2>9. Termination</h2>
        <h3>9.1 Your right to cancel</h3>
        <ul>
          <li>Cancel any time from your account settings</li>
          <li>No cancellation fees or penalties</li>
          <li>Download your data before cancelling</li>
        </ul>
        <h3>9.2 Our right to suspend or terminate</h3>
        <p>
          We may suspend or terminate your account for material breach of
          these terms, suspected fraud, a serious or repeated violation of
          the Acceptable use policy, or where required by law. We will give
          notice where reasonably possible.
        </p>
        <h3>9.3 Effect of termination</h3>
        <p>
          On termination, your right to use the Service ends, and we delete
          your data in accordance with the retention rules in the{" "}
          <a href="/privacy">Privacy policy</a> and the{" "}
          <a href="/dpa">Data Processing Addendum</a>.
        </p>

        <h2>10. Refunds</h2>
        <p>
          Our refund commitments are set out in the{" "}
          <a href="/refunds">Refunds policy</a> — a ninety-day trial, a
          fourteen-day post-purchase refund window, and full recognition of
          the Australian Consumer Law.
        </p>

        <h2>11. Limitation of liability</h2>
        <p>
          <strong>11.1 Cap.</strong> To the fullest extent permitted by
          law, the total aggregate liability of Tesserix arising out of or
          in connection with these terms or the Service is limited to the
          fees you paid Tesserix in the 12 months immediately preceding the
          event giving rise to the claim.
        </p>
        <p>
          <strong>11.2 Exclusions.</strong> Tesserix is not liable for
          indirect, incidental, special, consequential, or punitive damages;
          lost profits; lost revenue; lost data; or loss of goodwill, to
          the extent permitted by law.
        </p>
        <p>
          <strong>11.3 Non-excludable liability.</strong> Nothing in this
          clause limits liability for death or personal injury caused by
          negligence, fraud, wilful misconduct, or any liability that cannot
          be limited or excluded under applicable law — including under the
          Australian Consumer Law.
        </p>

        <h2>12. Australian Consumer Law</h2>
        <p>
          If you are a &ldquo;consumer&rdquo; within the meaning of the
          Australian Consumer Law (Schedule 2 of the Competition and
          Consumer Act 2010 (Cth)), nothing in these terms excludes,
          restricts, or modifies any consumer guarantee, right, or remedy
          conferred by the Australian Consumer Law. Our goods and services
          come with guarantees that cannot be excluded. For major failures
          with the Service, you are entitled to a refund or replacement and
          to compensation for any other reasonably foreseeable loss or
          damage.
        </p>

        <h2>13. Indemnity</h2>
        <p>
          You agree to indemnify and hold Tesserix harmless against claims
          arising from: (a) your content or products; (b) your breach of
          these terms; (c) your breach of applicable law, including
          consumer, privacy, tax, and intellectual property law. This does
          not apply to the extent the claim is caused by Tesserix&rsquo;s
          own negligence or wilful misconduct.
        </p>

        <h2>14. Force majeure</h2>
        <p>
          Neither party is liable for delay or failure to perform caused by
          events outside their reasonable control, including cloud provider
          outages, natural disasters, pandemics, acts of government,
          industrial disputes, internet outages, or acts of terrorism.
        </p>

        <h2>15. Governing law and dispute resolution</h2>
        <p>
          <strong>15.1 Governing law.</strong> These terms are governed by
          the laws of <strong>New South Wales, Australia</strong>, without
          regard to conflict-of-laws principles.
        </p>
        <p>
          <strong>15.2 Jurisdiction.</strong> The courts of New South Wales
          have exclusive jurisdiction, subject to any non-excludable
          consumer-protection forum rules in your jurisdiction.
        </p>
        <p>
          <strong>15.3 Dispute ladder.</strong> Before commencing
          proceedings, the parties will try to resolve any dispute in good
          faith for at least 30 days. If the dispute is not resolved, the
          parties will attempt mediation under the Rules of the Resolution
          Institute (Australia). Nothing in this clause prevents either
          party from seeking urgent interlocutory relief.
        </p>

        <h2>16. Changes to these terms</h2>
        <p>
          We may update these terms from time to time. Material changes are
          announced in-app at least 30 days before they take effect, where
          feasible. Continued use of the Service after changes take effect
          constitutes acceptance.
        </p>

        <h2>17. General</h2>
        <ul>
          <li><strong>Entire agreement:</strong> these terms together with the linked policies constitute the entire agreement between you and Tesserix.</li>
          <li><strong>Severability:</strong> if any clause is found unenforceable, the rest remains in effect.</li>
          <li><strong>No waiver:</strong> failure to enforce a right is not a waiver of it.</li>
          <li><strong>Assignment:</strong> you may not assign these terms without our written consent; we may assign to an affiliate or to a successor in a merger, acquisition, or sale of assets.</li>
          <li><strong>Notices:</strong> email is the primary notice channel for both parties.</li>
        </ul>

        <h2>18. Contact</h2>
        <ul>
          <li>
            <strong>Legal questions:</strong>{" "}
            <a href="mailto:legal@mark8ly.com">legal@mark8ly.com</a>
          </li>
          <li>
            <strong>Billing and refunds:</strong>{" "}
            <a href="mailto:support@mark8ly.com">support@mark8ly.com</a>
          </li>
          <li>
            <strong>Privacy:</strong>{" "}
            <a href="mailto:privacy@mark8ly.com">privacy@mark8ly.com</a>
          </li>
          <li>
            <strong>Security:</strong>{" "}
            <a href="mailto:security@mark8ly.com">security@mark8ly.com</a>
          </li>
        </ul>

        <h2>19. About Tesserix</h2>
        <p>
          Mark8ly is a product of <strong>Tesserix Pty Ltd</strong> (ACN 694
          070 865, ABN 59 694 070 865), registered in New South Wales,
          Australia. Operations are conducted from Mumbai, India and Sydney,
          Australia.
        </p>
      </Prose>
    </MarketingPage>
  );
}

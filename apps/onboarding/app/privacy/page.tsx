import { MailLink } from "@repo/ui/mail-link";
import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Privacy",
  description:
    "Plain-language privacy policy for Mark8ly. Tesserix Pty Ltd, New South Wales, Australia. APP-compliant, GDPR and UK GDPR honoured, CCPA/CPRA rights, DPDP aligned. We don&rsquo;t sell or share personal information for cross-context behavioural advertising.",
  alternates: { canonical: "/privacy" },
};

export default function PrivacyPolicyPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · April 2026"
        title={<>Privacy policy.</>}
        lede="Your data is yours. We only collect what we need to run the service and we&rsquo;re direct about how we use it."
      />

      <Prose>
        <p>
          At Mark8ly we believe your data is yours. We only collect what we
          need to provide the service, and we&rsquo;re transparent about how
          we use it.
        </p>

        <h2>1. Who we are</h2>
        <p>
          Mark8ly is an ecommerce platform that helps small businesses launch
          and manage online stores. Mark8ly is a product of{" "}
          <strong>Tesserix Pty Ltd</strong> (ACN 694 070 865, ABN 59 694 070
          865), registered in New South Wales, Australia. Day-to-day
          operations are conducted from Sydney, Australia.
          Tesserix Pty Ltd is the data controller for the personal
          information processed through the Mark8ly website and account.
        </p>
        <p>
          <strong>Contact:</strong>{" "}
          <MailLink email="privacy@mark8ly.com" /> ·{" "}
          <strong>Data Protection Officer:</strong>{" "}
          <MailLink email="dpo@mark8ly.com" />
        </p>

        <h2>2. Information we collect</h2>
        <h3>Information you give us</h3>
        <ul>
          <li>
            <strong>Account information:</strong> name, email, phone number,
            business name, and login credentials.
          </li>
          <li>
            <strong>Business information:</strong> store name, product
            details, descriptions, images, and pricing.
          </li>
          <li>
            <strong>Payment information:</strong> billing address and payment
            method details. We never store full card numbers — card data is
            handled by Stripe, Razorpay, and Cashfree Payments.
          </li>
          <li>
            <strong>Communications:</strong> messages you send us through
            email, chat, or support.
          </li>
        </ul>
        <h3>Information we collect automatically</h3>
        <ul>
          <li>
            <strong>Usage data:</strong> how you interact with the platform,
            features you use, pages you visit.
          </li>
          <li>
            <strong>Device information:</strong> IP address, browser type,
            operating system, device type.
          </li>
          <li>
            <strong>Cookies:</strong> a small set of cookies to keep you
            signed in and improve the service. See our{" "}
            <a href="/cookies">Cookie policy</a> for the full list.
          </li>
        </ul>

        <h2>3. How we use your information</h2>
        <h3>To provide the service</h3>
        <ul>
          <li>Create and manage your account</li>
          <li>Process transactions and send confirmations</li>
          <li>Run your online store</li>
          <li>Provide customer support</li>
        </ul>
        <h3>To improve the service</h3>
        <ul>
          <li>Understand how you use the platform</li>
          <li>Develop new features and improvements</li>
          <li>Fix bugs and technical issues</li>
        </ul>
        <p>
          We rely on the following lawful bases under GDPR and UK GDPR:
          performance of a contract (to deliver the service you&rsquo;ve
          signed up for), legitimate interests (to secure, improve, and
          support the service), legal obligation (tax and record-keeping
          laws), and consent (where we ask for it explicitly, e.g., optional
          analytics).
        </p>

        <h2>4. How we share your information</h2>
        <p>
          <strong>
            We don&rsquo;t sell your personal information, and we don&rsquo;t
            share it for cross-context behavioural advertising.
          </strong>{" "}
          We only share your data in a few limited situations:
        </p>
        <ul>
          <li>
            <strong>Sub-processors:</strong> vendors that run parts of the
            service on our behalf — hosting, payment processing, email
            delivery, source control. See the full list at{" "}
            <a href="/sub-processors">/sub-processors</a>.
          </li>
          <li>
            <strong>Your customers:</strong> the minimum needed to fulfil
            their orders.
          </li>
          <li>
            <strong>Legal requirements:</strong> when required by law or a
            valid court order; we push back on overbroad requests.
          </li>
          <li>
            <strong>Business transfers:</strong> in the event of a merger,
            acquisition, or sale of assets, with notice to you and the same
            protections in place.
          </li>
        </ul>

        <h2>5. Data security</h2>
        <p>We take security seriously:</p>
        <ul>
          <li>TLS 1.2+ encryption for every connection</li>
          <li>AES-256 at rest on managed Google Cloud services</li>
          <li>
            PCI DSS-compliant payment routing through Stripe, Razorpay, and
            Cashfree Payments
          </li>
          <li>APP-, GDPR-, UK GDPR-, CCPA/CPRA-, and DPDP-aligned data handling</li>
          <li>Regular dependency scanning and code review</li>
        </ul>
        <p>
          See our <a href="/security">Security page</a> for the full
          description of technical and organisational measures.
        </p>

        <h2>6. International transfers</h2>
        <p>
          Personal information may be processed in Australia, India, the
          European Economic Area, the United Kingdom, and the United States
          by Tesserix Pty Ltd and our sub-processors. Our primary processing
          region is <strong>asia-south1 (Mumbai)</strong>. Where we transfer
          personal information out of the EEA or UK to a country without an
          adequacy decision, we rely on the EU Standard Contractual Clauses
          and the UK International Data Transfer Addendum. Where we transfer
          personal information to India, we honour the Digital Personal Data
          Protection Act, 2023 where applicable.
        </p>

        <h2>7. Retention</h2>
        <ul>
          <li>
            <strong>Account data:</strong> retained for the life of the
            account plus 90 days after closure for wind-down.
          </li>
          <li>
            <strong>Billing records:</strong> retained for seven years to
            meet Australian tax and corporate-record obligations.
          </li>
          <li>
            <strong>Support correspondence:</strong> retained for two years.
          </li>
          <li>
            <strong>Backups:</strong> rotated on a schedule not exceeding 90
            days; deletion propagates on the next rotation.
          </li>
        </ul>
        <p>
          You may request earlier deletion under section 8.
        </p>

        <h2>8. Your rights</h2>
        <p>
          Regardless of where you live, you have the following rights over
          your personal information:
        </p>
        <ul>
          <li>
            <strong>Access:</strong> request a copy of your personal data.
          </li>
          <li>
            <strong>Correction:</strong> update or correct inaccurate data.
          </li>
          <li>
            <strong>Deletion:</strong> request deletion of your data, subject
            to legal retention obligations.
          </li>
          <li>
            <strong>Portability:</strong> receive your data in a
            machine-readable format.
          </li>
          <li>
            <strong>Withdraw consent:</strong> withdraw consent at any time
            where Processing is based on consent.
          </li>
          <li>
            <strong>Object or restrict:</strong> object to, or restrict,
            certain Processing based on legitimate interests.
          </li>
        </ul>
        <p>
          To exercise your rights, email{" "}
          <MailLink email="privacy@mark8ly.com" />. We
          respond within 30 days.
        </p>

        <h3>8.1 Region-specific rights</h3>
        <ul>
          <li>
            <strong>Australia (APPs):</strong> you may complain to us in
            writing; if unresolved, you may complain to the Office of the
            Australian Information Commissioner (OAIC) at{" "}
            <a href="https://www.oaic.gov.au" rel="nofollow">oaic.gov.au</a>.
          </li>
          <li>
            <strong>EEA (GDPR) &amp; UK (UK GDPR):</strong> you may lodge a
            complaint with your local supervisory authority in addition to
            contacting us.
          </li>
          <li>
            <strong>California (CCPA/CPRA):</strong> you have the right to
            know, delete, correct, and opt out of sale or sharing of
            personal information. We do not sell or share personal
            information for cross-context behavioural advertising, and we do
            not use sensitive personal information for purposes outside
            those permitted by CPRA.
          </li>
          <li>
            <strong>India (DPDP):</strong> you have rights of access,
            correction, erasure, grievance redressal, and nomination, per
            the Digital Personal Data Protection Act, 2023. Our grievance
            officer is reachable at{" "}
            <MailLink email="dpo@mark8ly.com" />.
          </li>
        </ul>

        <h2>9. Cookies</h2>
        <p>
          We use a short list of cookies for session management, preferences,
          and — where consent applies — analytics. See the full inventory,
          purposes, and opt-out instructions at{" "}
          <a href="/cookies">/cookies</a>.
        </p>

        <h2>10. Sub-processors</h2>
        <p>
          Mark8ly relies on a small set of vendors to run the service. The
          full list, their purpose, processing location, and transfer
          mechanism are published at{" "}
          <a href="/sub-processors">/sub-processors</a>, along with our
          change-notification commitment.
        </p>

        <h2>11. Breach notification</h2>
        <p>
          We notify affected users and regulators within{" "}
          <strong>72 hours</strong> of becoming aware of a personal data
          breach where required by applicable law.
        </p>

        <h2>12. Children</h2>
        <p>
          Mark8ly is not directed to children under 16. We do not knowingly
          collect personal information from children under 16. If you
          believe we&rsquo;ve received such information, email{" "}
          <MailLink email="privacy@mark8ly.com" /> and
          we will delete it.
        </p>

        <h2>13. Changes to this policy</h2>
        <p>
          We may update this policy from time to time. Material changes are
          announced in-app. The &ldquo;Last updated&rdquo; date at the top
          reflects the most recent change.
        </p>

        <h2>14. Contact us</h2>
        <ul>
          <li>
            <strong>General privacy:</strong>{" "}
            <MailLink email="privacy@mark8ly.com" />
          </li>
          <li>
            <strong>Data Protection Officer / grievance officer (India):</strong>{" "}
            <MailLink email="dpo@mark8ly.com" />
          </li>
          <li>
            <strong>Legal:</strong>{" "}
            <MailLink email="legal@mark8ly.com" />
          </li>
        </ul>

        <h2>15. About Tesserix</h2>
        <p>
          Mark8ly is a product of <strong>Tesserix Pty Ltd</strong> (ACN 694
          070 865, ABN 59 694 070 865), registered in New South Wales,
          Australia.
        </p>
      </Prose>
    </MarketingPage>
  );
}

import { MailLink } from "@repo/ui/mail-link";
import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Data Processing Addendum",
  description:
    "Mark8ly Data Processing Addendum. Auto-accepted on signup for merchants acting as controllers. GDPR Article 28 aligned.",
  alternates: { canonical: "/dpa" },
  robots: { index: false, follow: true },
};

export default function DpaPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · April 2026"
        title={<>Data Processing Addendum.</>}
        lede="Our contract when you act as a data controller and Mark8ly processes personal data on your behalf."
      />

      <Prose>
        <p>
          This Data Processing Addendum (&ldquo;<strong>DPA</strong>&rdquo;)
          forms part of the Mark8ly Terms of Service between{" "}
          <strong>Tesserix Pty Ltd</strong> (ACN 694 070 865, ABN 59 694 070
          865) (&ldquo;<strong>Processor</strong>&rdquo;, &ldquo;Mark8ly&rdquo;,
          &ldquo;we&rdquo;) and the merchant account holder (&ldquo;
          <strong>Controller</strong>&rdquo;, &ldquo;you&rdquo;). It applies
          whenever Mark8ly processes Personal Data that you, as Controller,
          have uploaded to or collected through the service. The DPA is
          auto-accepted when you create a Mark8ly account.
        </p>

        <h2>1. Definitions</h2>
        <p>
          Capitalised terms not defined here have the meaning given in the
          EU General Data Protection Regulation (&ldquo;<strong>GDPR</strong>
          &rdquo;). &ldquo;<strong>Personal Data</strong>&rdquo;, &ldquo;
          <strong>Processing</strong>&rdquo;, &ldquo;<strong>Controller
          </strong>&rdquo;, &ldquo;<strong>Processor</strong>&rdquo;, &ldquo;
          <strong>Sub-processor</strong>&rdquo;, and &ldquo;<strong>Data
          Subject</strong>&rdquo; have the GDPR meanings. &ldquo;<strong>
          Applicable Data Protection Law</strong>&rdquo; means all privacy
          laws applicable to the Processing, including GDPR, UK GDPR, the
          California Consumer Privacy Act as amended by the CPRA
          (&ldquo;CCPA&rdquo;), the Digital Personal Data Protection Act,
          2023 of India (&ldquo;DPDP&rdquo;), and the Privacy Act 1988 (Cth)
          of Australia with the Australian Privacy Principles
          (&ldquo;APPs&rdquo;).
        </p>

        <h2>2. Processing details (Annex I)</h2>
        <h3>2.1 Subject-matter and duration</h3>
        <p>
          Processing is carried out to provide the Mark8ly ecommerce
          platform service described in the Terms of Service. The duration
          is co-terminous with the main agreement plus the retention periods
          in section 9.
        </p>

        <h3>2.2 Nature and purpose</h3>
        <p>
          Hosting, storing, transmitting, displaying, backing up, and
          otherwise processing Personal Data as necessary to run a
          Controller&rsquo;s online store, fulfil orders, communicate with
          end customers, and generate analytics for the Controller.
        </p>

        <h3>2.3 Categories of Personal Data</h3>
        <ul>
          <li>Identification data (name, email, phone)</li>
          <li>Billing and shipping address</li>
          <li>Transaction data (order history, amounts — not full card numbers)</li>
          <li>Account credentials (managed through Google Identity Platform)</li>
          <li>Communications between Controller and end customers</li>
          <li>Usage data and IP addresses</li>
        </ul>

        <h3>2.4 Categories of Data Subjects</h3>
        <ul>
          <li>Controller&rsquo;s end customers</li>
          <li>Controller&rsquo;s staff with Mark8ly access</li>
          <li>Controller&rsquo;s suppliers or vendors, where recorded in the service</li>
        </ul>

        <h2>3. Processor obligations</h2>
        <p>
          Mark8ly will process Personal Data only:
        </p>
        <ul>
          <li>on documented instructions from the Controller — the Controller&rsquo;s use of the service is such an instruction</li>
          <li>to comply with applicable law; where law requires Processing beyond Controller instructions, we notify the Controller unless prohibited</li>
          <li>ensuring that personnel authorised to process Personal Data are bound by confidentiality</li>
        </ul>

        <h2>4. Security measures (Annex II)</h2>
        <p>
          Mark8ly implements appropriate technical and organisational
          measures described on our <a href="/security">Security page</a>,
          which is incorporated by reference. At minimum:
        </p>
        <ul>
          <li>TLS 1.2 or higher for all data in transit</li>
          <li>AES-256 encryption for data at rest on managed GCP services</li>
          <li>Least-privilege access with MFA required for staff production access</li>
          <li>Audit logging on production access</li>
          <li>Secrets managed through GCP Secret Manager and External Secrets Operator; no secrets in source</li>
          <li>Regular dependency scanning and container image signing</li>
        </ul>

        <h2>5. Sub-processors</h2>
        <p>
          The Controller gives general authorisation for Mark8ly to engage
          sub-processors. The current list is published at{" "}
          <a href="/sub-processors">/sub-processors</a>. Mark8ly will notify
          the Controller at least <strong>fourteen days</strong> before
          engaging a new sub-processor or materially changing how an
          existing one is used. The Controller may object within those
          fourteen days by emailing{" "}
          <MailLink email="privacy@mark8ly.com" />; if
          the parties cannot agree on an alternative, the Controller may
          terminate the affected part of the service without penalty.
        </p>

        <h2>6. Data Subject requests</h2>
        <p>
          Mark8ly will assist the Controller in responding to Data Subject
          requests (access, correction, deletion, portability, objection,
          restriction) using appropriate technical and organisational
          measures, taking into account the nature of the Processing. Most
          requests are servable by the Controller through the admin
          dashboard; for those that are not, email{" "}
          <MailLink email="privacy@mark8ly.com" />.
        </p>

        <h2>7. Breach notification</h2>
        <p>
          Mark8ly will notify the Controller <strong>without undue delay</strong>{" "}
          and in any event within <strong>72 hours</strong> after becoming
          aware of a Personal Data breach affecting the Controller&rsquo;s
          data, with enough information for the Controller to meet its own
          regulatory notification obligations.
        </p>

        <h2>8. International transfers</h2>
        <p>
          Mark8ly&rsquo;s primary processing region is asia-south1 (Mumbai).
          Where Personal Data is transferred out of the European Economic
          Area or the United Kingdom to a country that does not benefit from
          an adequacy decision, the parties incorporate by reference:
        </p>
        <ul>
          <li>the EU Standard Contractual Clauses (Module Two: Controller-to-Processor) approved by Commission Implementing Decision (EU) 2021/914, completed in accordance with Annex III below</li>
          <li>the UK International Data Transfer Addendum to the EU SCCs, Version B.1.0, where the transfer is subject to UK GDPR</li>
        </ul>
        <p>
          Sub-processors named on <a href="/sub-processors">/sub-processors</a>{" "}
          are parties to equivalent Standard Contractual Clauses with
          Mark8ly.
        </p>

        <h2>9. Deletion and return</h2>
        <p>
          On termination of the main agreement, the Controller may export
          Personal Data through the service for 30 days. After that window,
          Mark8ly will delete Personal Data from active systems within 30
          days and from backups on the next backup-rotation cycle (no more
          than 90 days), except where retention is required by applicable
          law (for example, Australian tax records).
        </p>

        <h2>10. Audit rights</h2>
        <p>
          On reasonable request and not more than once every 12 months
          (unless required by a regulator), Mark8ly will provide the
          Controller with a summary of its most recent independent security
          review. On-site audits require at least 30 days&rsquo; written
          notice, a signed non-disclosure agreement, and are conducted at the
          Controller&rsquo;s expense during business hours to minimise
          disruption. We prefer to satisfy audit obligations by providing
          existing reports wherever possible.
        </p>

        <h2>11. Controller obligations</h2>
        <p>
          The Controller warrants that it has the right to share Personal
          Data with Mark8ly, that it has a lawful basis for each Processing
          purpose, and that its privacy notices to Data Subjects cover the
          Processing Mark8ly performs on its behalf.
        </p>

        <h2>12. Liability</h2>
        <p>
          Liability under this DPA is subject to the limitations and
          exclusions in the main agreement, except that nothing in the main
          agreement or this DPA limits liability that cannot be excluded
          under Applicable Data Protection Law or the Australian Consumer
          Law.
        </p>

        <h2>13. Annex III — SCCs completion details</h2>
        <ul>
          <li><strong>Data exporter:</strong> the Controller (merchant account holder) as identified in the Mark8ly account record.</li>
          <li><strong>Data importer:</strong> Tesserix Pty Ltd (ACN 694 070 865), New South Wales, Australia. Contact: <MailLink email="dpo@mark8ly.com" />.</li>
          <li><strong>Module:</strong> Module Two — Transfer Controller to Processor.</li>
          <li><strong>Clause 7 (Docking):</strong> optional, not used.</li>
          <li><strong>Clause 9 (Sub-processors):</strong> General written authorisation, 14 days&rsquo; notice — see section 5.</li>
          <li><strong>Clause 11 (Redress):</strong> optional, not used.</li>
          <li><strong>Clause 17 (Governing law):</strong> laws of Ireland for SCC-governed disputes; disputes outside the SCCs governed by New South Wales, Australia per the Terms of Service.</li>
          <li><strong>Clause 18 (Forum):</strong> Irish courts for SCC-governed disputes.</li>
          <li><strong>Annex I.A, I.B, II:</strong> as set out in sections 2, 4, and 5 of this DPA.</li>
        </ul>

        <h2>14. Term and changes</h2>
        <p>
          This DPA is co-terminous with the main agreement. Mark8ly may
          update the DPA to reflect changes in Applicable Data Protection
          Law or our practices; we will post the new version on this page
          and, for material changes, announce it in-app at least 30 days in
          advance where feasible.
        </p>

        <h2>15. Governing law</h2>
        <p>
          Except where Clause 17 of the SCCs applies, this DPA is governed
          by the laws of New South Wales, Australia. The courts of New South
          Wales have exclusive jurisdiction, consistent with the main
          agreement.
        </p>

        <h2>16. Contact</h2>
        <ul>
          <li>
            <strong>Data Protection Officer:</strong>{" "}
            <MailLink email="dpo@mark8ly.com" />
          </li>
          <li>
            <strong>Privacy:</strong>{" "}
            <MailLink email="privacy@mark8ly.com" />
          </li>
          <li>
            <strong>Legal:</strong>{" "}
            <MailLink email="legal@mark8ly.com" />
          </li>
        </ul>
      </Prose>
    </MarketingPage>
  );
}

import { MailLink } from "@repo/ui/mail-link";
import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Sub-processors",
  description:
    "The vendors Mark8ly uses to run the service. Purpose, data category, location, and transfer mechanism for each.",
  alternates: { canonical: "/sub-processors" },
};

interface SubProcessor {
  vendor: string;
  purpose: string;
  data: string;
  location: string;
  transfer: string;
}

const SUB_PROCESSORS: ReadonlyArray<SubProcessor> = [
  {
    vendor: "Google Cloud Platform",
    purpose: "Hosting, Cloud SQL (PostgreSQL), Pub/Sub, Cloud Storage, Secret Manager, Google Identity Platform",
    data: "All customer and merchant data at rest, authentication tokens, event streams",
    location: "asia-south1 (Mumbai, primary); limited control-plane metadata in global GCP regions",
    transfer: "Standard Contractual Clauses; GCP privacy addendum; EU and UK addenda where required",
  },
  {
    vendor: "Cloudflare",
    purpose: "DNS, CDN, DDoS protection, Cloudflare Tunnel",
    data: "Request metadata, IP addresses, routing headers",
    location: "Global edge network; traffic terminated at the nearest edge",
    transfer: "Standard Contractual Clauses via Cloudflare DPA",
  },
  {
    vendor: "Stripe",
    purpose: "Payment processing, fraud detection",
    data: "Payment method details, billing address, transaction metadata — Mark8ly does not store full card numbers",
    location: "Global (Stripe regional processing)",
    transfer: "Standard Contractual Clauses; PCI DSS Level 1",
  },
  {
    vendor: "Razorpay",
    purpose: "Payment processing for Indian merchants and customers",
    data: "Payment method details, billing address, transaction metadata",
    location: "India",
    transfer: "Domestic (within India); DPDP-aligned",
  },
  {
    vendor: "Cashfree Payments",
    purpose: "Payment processing for Indian merchants and customers",
    data: "Payment method details, billing address, contact phone, transaction metadata",
    location: "India",
    transfer: "Domestic (within India); DPDP-aligned",
  },
  {
    vendor: "SendGrid (Twilio)",
    purpose: "Transactional email delivery (receipts, invites, password resets)",
    data: "Recipient email, message content, delivery status",
    location: "United States (primary processing)",
    transfer: "Standard Contractual Clauses via Twilio DPA",
  },
  {
    vendor: "Firebase Cloud Messaging",
    purpose: "Push notifications to Mark8ly mobile apps (admin, storefront)",
    data: "Device tokens, notification payloads (not typically Personal Data)",
    location: "Global (Google infrastructure)",
    transfer: "Standard Contractual Clauses via Google DPA",
  },
  {
    vendor: "GitHub",
    purpose: "Source control, CI/CD, container registry (GHCR)",
    data: "Source code, build metadata, container images — no customer data",
    location: "United States",
    transfer: "Standard Contractual Clauses via GitHub DPA",
  },
];

export default function SubProcessorsPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · April 2026"
        title={<>Sub-processors.</>}
        lede="The vendors we use to run Mark8ly, what they do, and where your data sits."
      />

      <Prose>
        <p>
          A <strong>sub-processor</strong> is a third party that processes
          personal data on our behalf to help us provide the service. We keep
          this list short on purpose and review it whenever we onboard a new
          vendor.
        </p>

        <h2>1. Current sub-processors</h2>
        <div style={{ overflowX: "auto" }}>
          <table
            style={{
              width: "100%",
              borderCollapse: "collapse",
              margin: "1.5rem 0",
              fontSize: "0.9375rem",
            }}
          >
            <thead>
              <tr>
                <th style={{ textAlign: "left", padding: "0.75rem 0.5rem", borderBottom: "1px solid var(--border-subtle, #e5e4e0)" }}>Vendor</th>
                <th style={{ textAlign: "left", padding: "0.75rem 0.5rem", borderBottom: "1px solid var(--border-subtle, #e5e4e0)" }}>Purpose</th>
                <th style={{ textAlign: "left", padding: "0.75rem 0.5rem", borderBottom: "1px solid var(--border-subtle, #e5e4e0)" }}>Data</th>
                <th style={{ textAlign: "left", padding: "0.75rem 0.5rem", borderBottom: "1px solid var(--border-subtle, #e5e4e0)" }}>Location</th>
                <th style={{ textAlign: "left", padding: "0.75rem 0.5rem", borderBottom: "1px solid var(--border-subtle, #e5e4e0)" }}>Transfer</th>
              </tr>
            </thead>
            <tbody>
              {SUB_PROCESSORS.map((sp) => (
                <tr key={sp.vendor}>
                  <td style={{ padding: "0.75rem 0.5rem", verticalAlign: "top", borderBottom: "1px solid var(--border-subtle, #eee)" }}>
                    <strong>{sp.vendor}</strong>
                  </td>
                  <td style={{ padding: "0.75rem 0.5rem", verticalAlign: "top", borderBottom: "1px solid var(--border-subtle, #eee)" }}>
                    {sp.purpose}
                  </td>
                  <td style={{ padding: "0.75rem 0.5rem", verticalAlign: "top", borderBottom: "1px solid var(--border-subtle, #eee)" }}>
                    {sp.data}
                  </td>
                  <td style={{ padding: "0.75rem 0.5rem", verticalAlign: "top", borderBottom: "1px solid var(--border-subtle, #eee)" }}>
                    {sp.location}
                  </td>
                  <td style={{ padding: "0.75rem 0.5rem", verticalAlign: "top", borderBottom: "1px solid var(--border-subtle, #eee)" }}>
                    {sp.transfer}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <h2>2. Primary processing region</h2>
        <p>
          The Mark8ly cluster runs in <strong>asia-south1 (Mumbai)</strong>.
          Customer data at rest — databases, object storage, event streams —
          lives in Mumbai by default. Some sub-processors above (payment
          providers, email delivery, push notifications, source control) are
          hosted elsewhere; their entries above name the region.
        </p>

        <h2>3. Transfer mechanisms</h2>
        <p>
          Where personal data is transferred out of the European Economic
          Area, the United Kingdom, or another region with data-transfer
          restrictions, we rely on the EU Standard Contractual Clauses
          (together with the UK International Data Transfer Addendum where
          applicable) signed with each sub-processor. Some vendors
          additionally qualify under adequacy decisions. Indian transfers
          honour the Digital Personal Data Protection Act, 2023 where
          applicable.
        </p>

        <h2>4. Change notification</h2>
        <p>
          We commit to updating this page at least{" "}
          <strong>fourteen days</strong> before onboarding a new sub-processor
          or materially changing how an existing one is used. Merchants who
          have signed our Data Processing Addendum may subscribe to change
          notifications by emailing{" "}
          <MailLink email="privacy@mark8ly.com" /> with
          the subject line &ldquo;sub-processor updates&rdquo;.
        </p>
        <p>
          Merchants who object to a new sub-processor may terminate the
          affected part of the service without penalty per the{" "}
          <a href="/dpa">Data Processing Addendum</a>.
        </p>

        <h2>5. Contact</h2>
        <ul>
          <li>
            <strong>Privacy / DPA questions:</strong>{" "}
            <MailLink email="privacy@mark8ly.com" />
          </li>
          <li>
            <strong>Data protection officer:</strong>{" "}
            <MailLink email="dpo@mark8ly.com" />
          </li>
        </ul>

        <h2>6. About Tesserix</h2>
        <p>
          Mark8ly is a product of <strong>Tesserix Pty Ltd</strong> (ACN 694
          070 865, ABN 59 694 070 865), registered in New South Wales,
          Australia.
        </p>
      </Prose>
    </MarketingPage>
  );
}

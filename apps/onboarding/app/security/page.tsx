import { MailLink } from "@repo/ui/mail-link";
import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Security",
  description:
    "How Mark8ly protects your data. Encryption, access controls, infrastructure, secure SDLC, incident response, and responsible disclosure.",
  alternates: { canonical: "/security" },
};

export default function SecurityPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · April 2026"
        title={<>Security.</>}
        lede="How we protect your store, your customers&rsquo; data, and ourselves. Concrete, not buzzwords."
      />

      <Prose>
        <p>
          We treat security as a product feature, not a checkbox. This page
          describes what we actually do today. Where a control is aspirational
          or still being rolled out, we say so plainly.
        </p>

        <h2>1. Encryption</h2>
        <ul>
          <li><strong>In transit:</strong> TLS 1.2 or higher on every public endpoint. mTLS between internal services via Istio service mesh.</li>
          <li><strong>At rest:</strong> AES-256 encryption on Cloud SQL, Cloud Storage, and persistent volumes, managed by Google Cloud Platform. Customer-managed encryption keys (CMEK) are available on request for enterprise accounts.</li>
          <li><strong>Secrets:</strong> Stored in Google Cloud Secret Manager and surfaced to workloads via the External Secrets Operator. No secrets live in source code or container images.</li>
        </ul>

        <h2>2. Access controls</h2>
        <ul>
          <li>Least-privilege IAM across GCP, Kubernetes, and application layers</li>
          <li>Multi-factor authentication required for every staff account with production access</li>
          <li>Production access is audit-logged; break-glass access triggers an alert</li>
          <li>Fine-grained authorisation inside the product is modelled in OpenFGA</li>
          <li>End-user authentication is handled by Google Identity Platform — password storage is Google&rsquo;s problem, not ours</li>
        </ul>

        <h2>3. Infrastructure</h2>
        <ul>
          <li>Google Kubernetes Engine (GKE) Autopilot for workload isolation and managed upgrades</li>
          <li>Knative Serving for scale-to-zero and concurrency-based autoscaling</li>
          <li>Istio service mesh with sidecar injection and default mTLS between services</li>
          <li>Cloud SQL PostgreSQL for relational data, with automated backups and point-in-time recovery</li>
          <li>Cloudflare in front of the stack for DDoS protection, bot management, and edge caching</li>
          <li>Primary processing region: <strong>asia-south1 (Mumbai)</strong></li>
        </ul>

        <h2>4. Secure development lifecycle</h2>
        <ul>
          <li>Mandatory code review on every change; no direct pushes to main branches</li>
          <li>Dependency and vulnerability scanning in CI</li>
          <li>Container images built with distroless or Alpine base, signed, and pushed to Google Artifact Registry with image retention rules</li>
          <li>No long-lived service account keys — GitHub Actions authenticates to GCP via Workload Identity Federation</li>
        </ul>

        <h2>5. Logging and monitoring</h2>
        <p>
          Application and infrastructure logs are centralised in Google Cloud
          Logging. Prometheus-compatible metrics and OpenTelemetry traces
          give us an alertable view of production health. Security-relevant
          events are routed to a separate audit trail.
        </p>

        <h2>6. Incident response</h2>
        <p>
          When a security incident affects customer data, we commit to
          notifying affected customers and regulators (where required by
          applicable law) within <strong>72 hours</strong> of becoming aware.
          Our incident-response runbook covers detection, containment,
          eradication, recovery, and post-incident review. We&rsquo;re always
          happy to walk through the runbook with enterprise customers under
          NDA.
        </p>

        <h2>7. Vulnerability disclosure</h2>
        <p>
          If you think you&rsquo;ve found a security vulnerability, please
          tell us. Email{" "}
          <MailLink email="security@mark8ly.com" /> with
          a description, reproduction steps, and — if you know — the impact.
        </p>
        <ul>
          <li>We acknowledge every report within <strong>72 hours</strong></li>
          <li>We give credit to reporters (with permission) in a public thanks page, coming soon</li>
          <li>We do not pursue legal action against researchers acting in good faith under the safe harbour described below</li>
        </ul>
        <p>
          <strong>Safe harbour:</strong> testing that stays within documented
          scope, avoids degradation of service, avoids accessing or
          modifying data that is not your own, and is promptly disclosed to
          us will be treated as authorised for the purposes of both the law
          and our Terms of Service.
        </p>

        <h2>8. Data residency and transfers</h2>
        <p>
          Customer data at rest is stored in asia-south1 (Mumbai). Some
          third-party sub-processors — for payments, email delivery, source
          control, and push notifications — are hosted elsewhere. See{" "}
          <a href="/sub-processors">/sub-processors</a> for the full list and
          transfer mechanisms.
        </p>

        <h2>9. Compliance posture</h2>
        <p>
          Mark8ly is <strong>not yet certified</strong> against SOC 2 or ISO
          27001. We&rsquo;re working toward a SOC 2 Type I engagement and
          will update this page when it completes. We align to the
          Australian Privacy Principles (APPs), GDPR, UK GDPR, CCPA/CPRA,
          and the Digital Personal Data Protection Act, 2023 of India. See
          our <a href="/privacy">Privacy policy</a> and{" "}
          <a href="/dpa">Data Processing Addendum</a> for details.
        </p>
        <p>
          Card payments are handled exclusively by Stripe, Razorpay, and
          Cashfree Payments;
          Mark8ly does not store full card numbers. PCI DSS obligations for
          payment data sit with those processors.
        </p>

        <h2>10. Contact</h2>
        <ul>
          <li>
            <strong>Vulnerability reports:</strong>{" "}
            <MailLink email="security@mark8ly.com" />
          </li>
          <li>
            <strong>Privacy &amp; DPA:</strong>{" "}
            <MailLink email="privacy@mark8ly.com" />
          </li>
          <li>
            <strong>General legal:</strong>{" "}
            <MailLink email="legal@mark8ly.com" />
          </li>
        </ul>

        <h2>11. About Tesserix</h2>
        <p>
          Mark8ly is a product of <strong>Tesserix Pty Ltd</strong> (ACN 694
          070 865, ABN 59 694 070 865), registered in New South Wales,
          Australia.
        </p>
      </Prose>
    </MarketingPage>
  );
}

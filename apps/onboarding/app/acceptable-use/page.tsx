import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Acceptable use",
  description:
    "What you can and can&rsquo;t do on Mark8ly. Prohibited goods and conduct, takedown process, appeals, and how to report abuse.",
  alternates: { canonical: "/acceptable-use" },
};

export default function AcceptableUsePage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · April 2026"
        title={<>Acceptable use.</>}
        lede="A short list of things you can&rsquo;t do on Mark8ly, and what happens if you try."
      />

      <Prose>
        <p>
          This Acceptable Use Policy applies to every Mark8ly account and to
          every person who uses a Mark8ly-hosted store — merchants, their
          staff, and their customers. It is part of our Terms of Service.
        </p>

        <h2>1. Prohibited content</h2>
        <p>You may not list, sell, distribute, or promote:</p>
        <ul>
          <li>Illegal goods or services under Australian, Indian, or any other applicable law</li>
          <li>Weapons, ammunition, or explosives</li>
          <li>Controlled substances, prescription medication, or drug paraphernalia</li>
          <li>Adult content without verifiable age-gating and all required local licences</li>
          <li>Counterfeit goods or items that infringe a third party&rsquo;s intellectual property</li>
          <li>Live animals, protected wildlife, or products derived from endangered species</li>
          <li>Stolen property or goods you do not have the right to sell</li>
          <li>Content that exploits minors, incites violence, or promotes hatred against a protected group</li>
        </ul>

        <h2>2. Prohibited conduct</h2>
        <p>You may not use Mark8ly to:</p>
        <ul>
          <li>Engage in fraud, deceptive practices, or money laundering</li>
          <li>Run schemes, pyramid or multi-level marketing arrangements, or any offering regulated as a financial product without the right licence</li>
          <li>Upload malware, run phishing campaigns, or attempt unauthorised access to any system or account</li>
          <li>Interfere with other merchants&rsquo; stores or the Mark8ly service itself (including denial-of-service, scraping beyond published rate limits, or reverse-engineering)</li>
          <li>Misrepresent your identity, business, or product provenance to customers or to us</li>
          <li>Send unsolicited bulk communications from a Mark8ly domain or infrastructure</li>
        </ul>

        <h2>3. Intellectual property</h2>
        <p>
          You are responsible for having the rights — yours or licensed — to
          every image, logo, font, description, product, and brand you upload.
          We honour valid takedown notices under the Copyright Act 1968 (Cth)
          and equivalent frameworks in other jurisdictions. See the takedown
          process in our <a href="/terms">Terms of Service</a>.
        </p>

        <h2>4. Enforcement</h2>
        <p>When we see a violation, our response scales with severity:</p>
        <ul>
          <li><strong>Warning</strong> — most first-time, low-severity issues get a written warning with a chance to fix</li>
          <li><strong>Suspension</strong> — repeated or material violations suspend the account while we investigate</li>
          <li><strong>Termination</strong> — fraud, illegal content, CSAM, or serious abuse skips the warning step and leads to immediate termination and, where required, reporting to the relevant authority</li>
        </ul>

        <h2>5. Appeals</h2>
        <p>
          If your account is suspended or terminated and you believe we got
          it wrong, email{" "}
          <a href="mailto:legal@mark8ly.com">legal@mark8ly.com</a> within 30
          days. Include your store name, the action we took, and why you think
          it was incorrect. We respond within 10 business days.
        </p>

        <h2>6. Reporting abuse</h2>
        <p>
          If you see a Mark8ly-hosted store that looks like it violates this
          policy — counterfeit goods, phishing, IP infringement, fraud — email{" "}
          <a href="mailto:abuse@mark8ly.com">abuse@mark8ly.com</a>. Include
          the store URL and a description of the issue.
        </p>

        <h2>7. Changes</h2>
        <p>
          We may update this policy as new abuse patterns emerge. Material
          changes are announced in-app and posted on this page with a new
          &ldquo;Last updated&rdquo; date.
        </p>

        <h2>8. About Tesserix</h2>
        <p>
          Mark8ly is a product of <strong>Tesserix Pty Ltd</strong> (ACN 694
          070 865, ABN 59 694 070 865), registered in New South Wales,
          Australia. Operations are conducted from Sydney, Australia.
        </p>
      </Prose>
    </MarketingPage>
  );
}

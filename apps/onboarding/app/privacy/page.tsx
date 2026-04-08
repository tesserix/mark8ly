import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Privacy",
};

export default function PrivacyPolicyPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · January 2026"
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
          Mark8ly is an e-commerce platform that helps small businesses
          launch and manage online stores. We&rsquo;re based in Mumbai, India,
          and serve customers globally.
        </p>
        <p>
          <strong>Contact:</strong>{" "}
          <a href="mailto:privacy@mark8ly.com">privacy@mark8ly.com</a>
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
            method details. We never store full card numbers.
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
            <strong>Cookies:</strong> we use cookies to keep you signed in
            and improve your experience.
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

        <h2>4. How we share your information</h2>
        <p>
          <strong>We don&rsquo;t sell your personal information. Ever.</strong>
        </p>
        <p>We only share your data in a few limited situations:</p>
        <ul>
          <li>
            <strong>Service providers:</strong> payment processors, cloud
            hosting, email delivery.
          </li>
          <li>
            <strong>Your customers:</strong> the minimum needed to fulfil
            their orders.
          </li>
          <li>
            <strong>Legal requirements:</strong> when required by law or a
            court order.
          </li>
          <li>
            <strong>Business transfers:</strong> in the event of a merger or
            acquisition, with notice to you.
          </li>
        </ul>

        <h2>5. Data security</h2>
        <p>We take security seriously:</p>
        <ul>
          <li>TLS encryption for every connection</li>
          <li>PCI-compliant payment routing</li>
          <li>GDPR-aligned data handling</li>
          <li>Regular third-party security reviews</li>
        </ul>

        <h2>6. Your rights</h2>
        <p>You have control over your data:</p>
        <ul>
          <li>
            <strong>Access:</strong> request a copy of your personal data.
          </li>
          <li>
            <strong>Correction:</strong> update or correct inaccurate data.
          </li>
          <li>
            <strong>Deletion:</strong> request deletion of your data.
          </li>
          <li>
            <strong>Portability:</strong> receive your data in a
            machine-readable format.
          </li>
          <li>
            <strong>Withdraw consent:</strong> withdraw consent at any time.
          </li>
        </ul>
        <p>
          To exercise your rights, email{" "}
          <a href="mailto:privacy@mark8ly.com">privacy@mark8ly.com</a>. We
          respond within 30 days.
        </p>

        <h2>7. Contact us</h2>
        <p>Questions about this privacy policy?</p>
        <ul>
          <li>
            <strong>General privacy:</strong>{" "}
            <a href="mailto:privacy@mark8ly.com">privacy@mark8ly.com</a>
          </li>
          <li>
            <strong>Data protection officer:</strong>{" "}
            <a href="mailto:dpo@mark8ly.com">dpo@mark8ly.com</a>
          </li>
        </ul>
      </Prose>
    </MarketingPage>
  );
}

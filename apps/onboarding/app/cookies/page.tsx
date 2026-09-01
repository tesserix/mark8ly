import { MailLink } from "@repo/ui/mail-link";
import {
  MarketingPage,
  PageHero,
  Prose,
} from "@/components/marketing/primitives";

export const metadata = {
  title: "Cookies",
  description:
    "How Mark8ly uses cookies. Categories, third-party cookies, and how to opt out. Plain-language, no dark patterns.",
  alternates: { canonical: "/cookies" },
};

export default function CookiePolicyPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Last updated · April 2026"
        title={<>Cookies.</>}
        lede="The small text files we use, why, and how to turn them off."
      />

      <Prose>
        <p>
          A cookie is a small text file a website stores in your browser to
          remember something about you — that you&rsquo;re signed in, which
          currency you prefer, whether you&rsquo;ve seen a particular prompt
          before. We use cookies for a small number of specific purposes,
          described below.
        </p>

        <h2>1. Categories we use</h2>

        <h3>Strictly necessary</h3>
        <p>
          These cookies are required for Mark8ly to work. They keep you
          signed in, protect against cross-site request forgery, and balance
          load across our servers. They cannot be disabled without breaking
          the service.
        </p>
        <ul>
          <li><strong>Session</strong> — identifies your authenticated session</li>
          <li><strong>CSRF token</strong> — prevents cross-site request forgery</li>
          <li><strong>Tenant context</strong> — routes you to the right store backend</li>
        </ul>

        <h3>Functional</h3>
        <p>
          These cookies remember preferences that make the experience
          smoother. You can disable them in your browser; the service will
          still work but will feel less tailored.
        </p>
        <ul>
          <li><strong>Preferences</strong> — language, region, currency</li>
          <li><strong>UI state</strong> — remembered dashboard layout and dismissed prompts</li>
        </ul>

        <h3>Analytics</h3>
        <p>
          We are working toward using a single, privacy-respecting analytics
          cookie so we can understand how the product is used in aggregate
          and make it better. Where consent is required by law (EU, UK, and
          other regions with similar rules), no analytics cookie is set
          until you opt in. A consent banner for those regions is rolling
          out in 2026.
        </p>

        <h2>2. Third-party cookies</h2>
        <p>
          Some cookies are set by services that run on our behalf. We keep
          this list short and review it when a new vendor is added.
        </p>
        <ul>
          <li><strong>Google Identity Platform</strong> — authentication; a short-lived token cookie is set when you sign in</li>
          <li><strong>Cloudflare</strong> — DDoS protection and routing; a short-lived anti-bot cookie may be set on some requests</li>
          <li><strong>Stripe</strong> — when you reach a checkout page powered by Stripe, Stripe sets its own cookies for fraud detection. Stripe&rsquo;s cookie policy applies in that context.</li>
        </ul>

        <h2>3. How to opt out</h2>
        <p>
          You can block or delete cookies in your browser settings. The
          instructions differ by browser:
        </p>
        <ul>
          <li><strong>Chrome</strong> — Settings → Privacy and security → Cookies and other site data</li>
          <li><strong>Firefox</strong> — Settings → Privacy &amp; Security → Cookies and Site Data</li>
          <li><strong>Safari</strong> — Settings → Privacy → Manage Website Data</li>
          <li><strong>Edge</strong> — Settings → Cookies and site permissions → Cookies and site data</li>
        </ul>
        <p>
          Blocking strictly-necessary cookies will prevent you from signing
          in to Mark8ly.
        </p>

        <h2>4. Consent posture</h2>
        <p>
          Strictly-necessary cookies are set without consent because the
          service cannot run without them. Analytics cookies — when we enable
          them — require consent where applicable law (GDPR, UK GDPR, ePrivacy
          Directive) requires it. Functional cookies ride on your continued
          use of the service.
        </p>

        <h2>5. Changes</h2>
        <p>
          When we add or remove a cookie in a way that affects you, we update
          this page and note it in the change log. For material changes we
          also announce it in-app.
        </p>

        <h2>6. Contact</h2>
        <p>
          Questions about cookies: email{" "}
          <MailLink email="privacy@mark8ly.com" />.
        </p>

        <h2>7. About Tesserix</h2>
        <p>
          Mark8ly is a product of <strong>Tesserix Pty Ltd</strong> (ACN 694
          070 865, ABN 59 694 070 865), registered in New South Wales,
          Australia. Primary processing region: asia-south1 (Mumbai).
        </p>
      </Prose>
    </MarketingPage>
  );
}

"use client";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

export default function SecurityCompliancePage() {
  return (
    <div className="min-h-screen bg-background">
      <Header />

      <section className="pt-32 pb-12 px-6">
        <div className="max-w-3xl mx-auto">
          <h1 className="font-serif text-4xl sm:text-5xl font-medium tracking-tight mb-4 text-foreground">
            Security &amp; Compliance
          </h1>
          <p className="text-foreground-secondary">Last Updated: February 2026</p>
        </div>
      </section>

      <section className="pb-20 px-6">
        <div className="max-w-3xl mx-auto">
          <div className="prose prose-lg text-foreground-secondary space-y-8">
            <div className="p-6 bg-warm-50 rounded-xl border border-warm-200">
              <p className="text-foreground leading-relaxed m-0">
                At mark8ly, we take security seriously. This policy explains
                how we protect your account, your data, and your business — and
                what we expect from you in return.
              </p>
            </div>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">1. Account Security</h2>
              <p className="leading-relaxed">
                Every merchant account on mark8ly is protected by multi-factor
                authentication (MFA). This is not optional — it&apos;s required
                for all users.
              </p>
              <ul className="list-disc pl-6 space-y-2 mt-4">
                <li><strong>MFA Required:</strong> All merchant accounts must have MFA enabled</li>
                <li><strong>Passkey Support:</strong> FIDO2/WebAuthn passwordless, phishing-resistant login</li>
                <li><strong>TOTP Authenticator:</strong> Use any authenticator app to generate one-time passwords</li>
                <li><strong>Backup Codes:</strong> Stored securely as your recovery lifeline</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">2. Password Policy</h2>
              <ul className="list-disc pl-6 space-y-2 mt-4">
                <li>Minimum 10 characters in length</li>
                <li>Mix of uppercase, lowercase, numbers, and symbols</li>
                <li>No common dictionary words or predictable patterns</li>
                <li>Cannot reuse any of your last 5 passwords</li>
                <li>All passwords are hashed using bcrypt — never stored in plaintext</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">3. Digital Theft Protection</h2>
              <ul className="list-disc pl-6 space-y-2 mt-4">
                <li><strong>TLS 1.3 Encryption:</strong> All data in transit encrypted</li>
                <li><strong>PCI DSS Level 1:</strong> Payment processing meets the highest standard</li>
                <li><strong>Rate Limiting:</strong> Login and API endpoints protected from abuse</li>
                <li><strong>Brute-Force Lockout:</strong> Accounts temporarily locked on repeated failed attempts</li>
                <li><strong>Suspicious Login Monitoring:</strong> Anomalous logins are flagged automatically</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">4. Business Safeguards</h2>
              <ul className="list-disc pl-6 space-y-2 mt-4">
                <li><strong>Data Backups:</strong> Daily automated with point-in-time recovery</li>
                <li><strong>Tenant Isolation:</strong> Your data is logically isolated from other merchants</li>
                <li><strong>Penetration Testing:</strong> Regular security assessments</li>
                <li><strong>99.9% Uptime SLA</strong> for production services</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">5. Incident Response</h2>
              <p className="leading-relaxed">
                If you believe your account has been compromised:
              </p>
              <ol className="list-decimal pl-6 space-y-2 mt-4">
                <li>Change your password from a trusted device</li>
                <li>Revoke all active sessions from your security settings</li>
                <li>Review recent activity in your audit log</li>
                <li>
                  Contact our security team at{" "}
                  <a href="mailto:security@mark8ly.com" className="text-primary hover:underline">
                    security@mark8ly.com
                  </a>
                </li>
              </ol>
              <div className="mt-4 p-4 bg-warm-50 border border-warm-200 rounded-lg">
                <p className="m-0">
                  <strong>Breach Notification:</strong> In the event of a
                  confirmed data breach, we will notify affected users within
                  72 hours, in compliance with GDPR.
                </p>
              </div>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">6. Contact</h2>
              <div className="mt-4 p-4 bg-warm-50 border border-warm-200 rounded-lg">
                <p className="m-0">
                  <strong>Security Team:</strong>{" "}
                  <a href="mailto:security@mark8ly.com" className="text-primary hover:underline">
                    security@mark8ly.com
                  </a>
                </p>
                <p className="mt-2 mb-0">
                  <strong>General Support:</strong>{" "}
                  <a href="mailto:support@mark8ly.com" className="text-primary hover:underline">
                    support@mark8ly.com
                  </a>
                </p>
              </div>
            </section>

            <section className="mt-12 p-6 bg-sage-50 rounded-xl border border-sage-200">
              <p className="text-sage-700 font-medium m-0 text-center">
                By using mark8ly, you agree to follow this Security &amp;
                Compliance policy.
              </p>
            </section>
          </div>
        </div>
      </section>

      <Footer />
    </div>
  );
}

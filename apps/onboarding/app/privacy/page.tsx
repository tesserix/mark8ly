"use client";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

export default function PrivacyPolicyPage() {
  return (
    <div className="min-h-screen bg-background">
      <Header />

      <section className="pt-32 pb-12 px-6">
        <div className="max-w-4xl mx-auto">
          <div className="inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white/82 px-4 py-2 text-sm font-medium text-foreground-secondary shadow-sm mb-6">
            Plain-language policy
          </div>
          <h1 className="font-serif text-4xl sm:text-5xl font-medium tracking-tight mb-4 text-foreground">
            Privacy Policy
          </h1>
          <p className="text-foreground-secondary">Last Updated: January 2026</p>
        </div>
      </section>

      <section className="pb-20 px-6">
        <div className="max-w-4xl mx-auto">
          <div className="rounded-[2rem] border border-warm-200/90 bg-white/90 p-6 shadow-[0_20px_60px_rgba(43,38,34,0.08)] sm:p-8">
            <div className="prose prose-lg text-foreground-secondary space-y-8">
              <div className="p-6 bg-warm-50 rounded-xl border border-warm-200">
                <p className="text-foreground leading-relaxed m-0">
                  At mark8ly, we believe your data is yours. We only collect what
                  we need to provide you with a great service, and we&apos;re
                  transparent about how we use it.
                </p>
              </div>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">1. Who We Are</h2>
              <p className="leading-relaxed">
                mark8ly is an e-commerce platform that helps small businesses
                launch and manage online stores. We&apos;re based in India and
                serve customers globally.
              </p>
              <p className="leading-relaxed mt-4">
                <strong>Contact:</strong> privacy@mark8ly.com
              </p>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">2. Information We Collect</h2>
              <h3 className="font-medium text-foreground text-lg mt-6 mb-3">Information You Give Us</h3>
              <ul className="list-disc pl-6 space-y-2">
                <li><strong>Account Information:</strong> Name, email address, phone number, business name, and login credentials</li>
                <li><strong>Business Information:</strong> Store name, product details, descriptions, images, and pricing</li>
                <li><strong>Payment Information:</strong> Billing address and payment method details (we never store full card details)</li>
                <li><strong>Communications:</strong> Messages you send us through email, chat, or support</li>
              </ul>
              <h3 className="font-medium text-foreground text-lg mt-6 mb-3">Information We Collect Automatically</h3>
              <ul className="list-disc pl-6 space-y-2">
                <li><strong>Usage Data:</strong> How you interact with our platform, features you use, pages you visit</li>
                <li><strong>Device Information:</strong> IP address, browser type, operating system, device type</li>
                <li><strong>Cookies:</strong> We use cookies to improve your experience</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">3. How We Use Your Information</h2>
              <h3 className="font-medium text-foreground text-lg mt-6 mb-3">To Provide Our Service</h3>
              <ul className="list-disc pl-6 space-y-2">
                <li>Create and manage your account</li>
                <li>Process transactions and send confirmations</li>
                <li>Enable your online store functionality</li>
                <li>Provide customer support</li>
              </ul>
              <h3 className="font-medium text-foreground text-lg mt-6 mb-3">To Improve Our Service</h3>
              <ul className="list-disc pl-6 space-y-2">
                <li>Understand how you use our platform</li>
                <li>Develop new features and improvements</li>
                <li>Fix bugs and technical issues</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">4. How We Share Your Information</h2>
              <div className="p-4 bg-sage-50 border border-sage-200 rounded-lg mb-6">
                <p className="text-sage-700 font-medium m-0">
                  We don&apos;t sell your personal information. Period.
                </p>
              </div>
              <p className="leading-relaxed">We only share your data in limited circumstances:</p>
              <ul className="list-disc pl-6 space-y-2 mt-4">
                <li><strong>Service Providers:</strong> Payment processors, cloud hosting, email services</li>
                <li><strong>Your Customers:</strong> Information necessary to fulfill orders</li>
                <li><strong>Legal Requirements:</strong> When required by law or court order</li>
                <li><strong>Business Transfers:</strong> In case of merger or acquisition (with notice)</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">5. Data Security</h2>
              <p className="leading-relaxed">We take security seriously:</p>
              <ul className="list-disc pl-6 space-y-2 mt-4">
                <li>SSL/TLS encryption for all data transmission</li>
                <li>PCI DSS compliant payment processing</li>
                <li>GDPR compliance for data protection</li>
                <li>Regular security audits</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">6. Your Rights</h2>
              <p className="leading-relaxed mb-4">You have control over your data:</p>
              <ul className="list-disc pl-6 space-y-2">
                <li><strong>Access:</strong> Request a copy of your personal data</li>
                <li><strong>Correction:</strong> Update or correct inaccurate information</li>
                <li><strong>Deletion:</strong> Request deletion of your data</li>
                <li><strong>Portability:</strong> Receive your data in a machine-readable format</li>
                <li><strong>Withdraw Consent:</strong> Withdraw consent anytime</li>
              </ul>
              <p className="leading-relaxed mt-4">
                To exercise your rights, email{" "}
                <a href="mailto:privacy@mark8ly.com" className="text-primary hover:underline">
                  privacy@mark8ly.com
                </a>
                . We&apos;ll respond within 30 days.
              </p>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">7. Contact Us</h2>
              <p className="leading-relaxed">Questions about this Privacy Policy?</p>
              <div className="mt-4 p-4 bg-warm-50 border border-warm-200 rounded-lg">
                <p className="m-0">
                  <strong>Email:</strong>{" "}
                  <a href="mailto:privacy@mark8ly.com" className="text-primary hover:underline">
                    privacy@mark8ly.com
                  </a>
                </p>
                <p className="mt-2 mb-0">
                  <strong>Data Protection Officer:</strong>{" "}
                  <a href="mailto:dpo@mark8ly.com" className="text-primary hover:underline">
                    dpo@mark8ly.com
                  </a>
                </p>
              </div>
            </section>
            </div>
          </div>
        </div>
      </section>

      <Footer />
    </div>
  );
}

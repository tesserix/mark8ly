"use client";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

export default function TermsOfServicePage() {
  return (
    <div className="min-h-screen bg-background">
      <Header />

      <section className="pt-32 pb-12 px-6">
        <div className="max-w-3xl mx-auto">
          <h1 className="font-serif text-4xl sm:text-5xl font-medium tracking-tight mb-4 text-foreground">
            Terms of Service
          </h1>
          <p className="text-foreground-secondary">Last Updated: January 2026</p>
        </div>
      </section>

      <section className="pb-20 px-6">
        <div className="max-w-3xl mx-auto">
          <div className="prose prose-lg text-foreground-secondary space-y-8">
            <div className="p-6 bg-warm-50 rounded-xl border border-warm-200">
              <p className="text-foreground leading-relaxed m-0">
                Welcome to mark8ly. These Terms of Service govern your use of
                our platform. By using mark8ly, you agree to these Terms.
                We&apos;ve written these in plain language, but they&apos;re
                still a legal agreement.
              </p>
            </div>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">1. Accepting These Terms</h2>
              <p className="leading-relaxed">
                By creating a mark8ly account or using our services, you agree
                to these Terms of Service, our Privacy Policy, and our
                Acceptable Use Policy.
              </p>
              <p className="leading-relaxed mt-4">
                <strong>Age Requirement:</strong> You must be at least 18 years
                old to use mark8ly.
              </p>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">2. Our Service</h2>
              <h3 className="font-medium text-foreground text-lg mt-6 mb-3">What We Provide</h3>
              <ul className="list-disc pl-6 space-y-2">
                <li>Store hosting and infrastructure</li>
                <li>Product management tools</li>
                <li>Payment processing integration</li>
                <li>Order management system</li>
                <li>24/7 human customer support</li>
              </ul>
              <h3 className="font-medium text-foreground text-lg mt-6 mb-3">What We Don&apos;t Provide</h3>
              <ul className="list-disc pl-6 space-y-2">
                <li>We don&apos;t manufacture, ship, or fulfill your products</li>
                <li>We don&apos;t provide legal, tax, or business advice</li>
                <li>We don&apos;t guarantee sales or revenue</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">3. Pricing and Payments</h2>
              <div className="p-4 bg-sage-50 border border-sage-200 rounded-lg mb-6">
                <p className="text-sage-700 m-0">
                  <strong>Free Trial:</strong> 12 months completely free. No
                  credit card required.
                </p>
                <p className="text-sage-700 mt-2 mb-0">
                  <strong>After Free Period:</strong> Simple flat pricing. No
                  transaction fees. Cancel anytime.
                </p>
              </div>
              <h3 className="font-medium text-foreground text-lg mt-6 mb-3">Refunds</h3>
              <p className="leading-relaxed">
                We offer refunds within 14 days of purchase. Contact
                support@mark8ly.com. After 14 days, charges are non-refundable,
                but you can cancel anytime.
              </p>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">4. Acceptable Use Policy</h2>
              <h3 className="font-medium text-foreground text-lg mt-6 mb-3">You May Not Use mark8ly To</h3>
              <ul className="list-disc pl-6 space-y-2">
                <li>Sell illegal, counterfeit, or restricted items</li>
                <li>Engage in fraudulent activity, schemes, or money laundering</li>
                <li>Upload malware or attempt unauthorized access</li>
                <li>Interfere with other merchants&apos; stores</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">5. Termination</h2>
              <h3 className="font-medium text-foreground text-lg mt-6 mb-3">Your Right to Cancel</h3>
              <ul className="list-disc pl-6 space-y-2">
                <li>Cancel anytime through your account settings</li>
                <li>No cancellation fees or penalties</li>
                <li>Download your data before canceling</li>
              </ul>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">6. Limitation of Liability</h2>
              <p className="leading-relaxed">
                To the fullest extent permitted by law, mark8ly&apos;s total
                liability is limited to the amount you paid us in the 12 months
                before the claim arose.
              </p>
            </section>

            <section>
              <h2 className="font-serif text-2xl font-medium text-foreground mb-4">7. Contact Us</h2>
              <div className="mt-4 p-4 bg-warm-50 border border-warm-200 rounded-lg">
                <p className="m-0">
                  <strong>Legal Questions:</strong>{" "}
                  <a href="mailto:legal@mark8ly.com" className="text-primary hover:underline">
                    legal@mark8ly.com
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
                Thank you for choosing mark8ly. We&apos;re honored to be part
                of your business journey.
              </p>
            </section>
          </div>
        </div>
      </section>

      <Footer />
    </div>
  );
}

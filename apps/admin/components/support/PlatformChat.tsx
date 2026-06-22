"use client";

import { OttoWidget } from "@tesserix/otto-widget";

import "@tesserix/otto-widget/styles/otto.css";
// Local override (loaded after the widget CSS): makes the post-close
// feedback survey scrollable so the Submit button is always reachable.
import "./otto-widget-overrides.css";

/**
 * PlatformChat is the admin-side chat surface to the Tesserix
 * platform-support team. The widget is identical to the customer
 * storefront widget — only the proxy (apiBaseUrl) and the tenantId
 * differ so the conversation routes to the `platform` slm-router
 * tenant + the Tesserix employee inbox at tesserix.app.
 *
 * The admin's identity is forwarded by the proxy (`X-User-Id` etc.)
 * so the OTP step is always skipped here. We pass `customerName`
 * and `customerEmail` props as well so the widget can render the
 * intake form correctly without prompting for them.
 */
interface PlatformChatProps {
  adminEmail?: string;
  adminName?: string;
  tenantName?: string;
}

export function PlatformChat({
  adminEmail,
  adminName,
  tenantName,
}: PlatformChatProps) {
  return (
    <OttoWidget
      apiBaseUrl="/api/otto-platform"
      productName="Tesserix Support"
      tenantId="platform"
      customerName={adminName}
      customerEmail={adminEmail}
      customerId={adminEmail}
      // Platform support gets its own intake reasons. Quick-question
      // stays at the top so admins can ask one-liners without the
      // structured form.
      reasons={[
        { value: "general_question", label: "Ask a quick question", requiresStatus: false },
        { value: "billing", label: "Billing or invoices" },
        { value: "domains", label: "Custom domain / DNS" },
        { value: "payments_setup", label: "Payments / Stripe / Razorpay" },
        { value: "incident", label: "Outage or incident" },
        { value: "kyc_compliance", label: "KYC or compliance" },
        { value: "feature_request", label: "Feature request" },
        { value: "other", label: "Something else" },
      ]}
      statusPlaceholder={
        tenantName
          ? `e.g. ${tenantName} checkout returning 500`
          : "e.g. Checkout returning 500 since 09:00 UTC"
      }
      launcherLabel="Chat with Tesserix"
    />
  );
}

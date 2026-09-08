import type { Metadata } from "next";

import { AdminPage, PageSection } from "@/components/layout";
import { ssoCopy } from "@/lib/copy/sso";

import { SSOClient } from "./SSOClient";

export const metadata: Metadata = { title: "Single sign-on" };

export default function SSOSettingsPage() {
  return (
    <AdminPage
      eyebrow={ssoCopy.eyebrow}
      title={ssoCopy.title}
      description={ssoCopy.description}
    >
      <PageSection
        title={ssoCopy.sectionTitle}
        description={ssoCopy.sectionDescription}
      >
        <SSOClient />
      </PageSection>
    </AdminPage>
  );
}

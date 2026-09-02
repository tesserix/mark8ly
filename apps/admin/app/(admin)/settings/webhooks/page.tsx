import { AdminPage, ReadOnlyNotice } from "@/components/layout";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listWebhooks } from "@/lib/api/webhooks";
import { WebhooksSettingsClient } from "@/components/settings/WebhooksSettingsClient";

/**
 * /settings/webhooks — outbound webhook subscriptions (#562 task 9).
 *
 * Available on every plan; there is no gate here to remove.
 */
export default async function WebhooksSettingsPage() {
  const { role, userId, tenantId, currentStore } = await getServerSessionContext();

  const editable = canEditSettings(role);

  return (
    <AdminPage
      eyebrow="Developer"
      title="Webhooks"
      description="Send order, return, product, and category events to your own endpoint the moment they happen. Sign every request with a secret only you and Mark8ly ever see."
      readOnlyNotice={!editable && role ? <ReadOnlyNotice role={role} /> : undefined}
    >
      {currentStore ? (
        <WebhooksSettingsContent
          storeId={currentStore.id}
          userId={userId}
          tenantId={tenantId}
          editable={editable}
        />
      ) : (
        <p className="text-sm text-danger">
          No store found. Please create a store before configuring webhooks.
        </p>
      )}
    </AdminPage>
  );
}

async function WebhooksSettingsContent({
  storeId,
  userId,
  tenantId,
  editable,
}: {
  storeId: string;
  userId: string;
  tenantId: string;
  editable: boolean;
}) {
  const webhooks = await listWebhooks(storeId, { userId, tenantId });
  return <WebhooksSettingsClient webhooks={webhooks} editable={editable} />;
}

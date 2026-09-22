"use server";

// apps/admin/app/products/import/actions.ts
//
// Server actions for CSV import. Separated from the main products
// actions.ts to keep route-scoped concerns isolated.

import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";

import {
  submitCsvImport,
  cancelCsvImport,
  type SessionHeaders,
} from "@/lib/api/csvImports";

export interface CsvActionResult {
  ok: boolean;
  error?: { code: string; message: string };
  jobId?: string;
}

async function readSession(): Promise<SessionHeaders | null> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!userId || !tenantId) return null;
  return { userId, tenantId };
}

/**
 * Resolve the acting store from the session, never from the caller.
 *
 * The upload action used to take a storeId argument and the page passed
 * the literal string "__STORE_ID__", so every upload 404'd — the feature
 * had never worked (#881). A client component cannot know the store id,
 * and letting it supply one is the wrong shape anyway: the session is the
 * authority on which store the merchant is acting in.
 *
 * Mirrors the resolution the csv-imports BFF route already does.
 */
async function resolveStoreId(): Promise<string> {
  const h = await headers();
  const fromSession = h.get("x-session-store-id") ?? "";
  if (fromSession) return fromSession;

  const tenantId = h.get("x-session-tenant-id") ?? "";
  if (!tenantId) return "";
  const { listStoresByTenant } = await import("@/lib/api/platform-api");
  const stores = await listStoresByTenant(tenantId).catch(() => []);
  return stores[0]?.id ?? "";
}

const NO_STORE = {
  ok: false as const,
  error: {
    code: "no_store",
    message:
      "We couldn't work out which store to import into. Reload and try again.",
  },
};

export async function submitCsvImportAction(
  formData: FormData,
): Promise<CsvActionResult> {
  const session = await readSession();
  if (!session) {
    return {
      ok: false,
      error: {
        code: "no_session",
        message: "Your session has expired. Please sign in again.",
      },
    };
  }

  const file = formData.get("file");
  if (!(file instanceof File) || file.size === 0) {
    return {
      ok: false,
      error: {
        code: "no_file",
        message: "Please select a CSV file to upload.",
      },
    };
  }

  const storeId = await resolveStoreId();
  if (!storeId) return NO_STORE;

  const result = await submitCsvImport(session, storeId, file);
  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath("/products/import");
  redirect(`/products/import/${result.data.id}`);
}

export async function cancelCsvImportAction(
  jobId: string,
): Promise<CsvActionResult> {
  const session = await readSession();
  if (!session) {
    return {
      ok: false,
      error: {
        code: "no_session",
        message: "Your session has expired. Please sign in again.",
      },
    };
  }

  const storeId = await resolveStoreId();
  if (!storeId) return NO_STORE;

  const result = await cancelCsvImport(session, storeId, jobId);
  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath(`/products/import/${jobId}`);
  return { ok: true };
}

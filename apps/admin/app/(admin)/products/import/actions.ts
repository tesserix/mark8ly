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

export async function submitCsvImportAction(
  storeId: string,
  formData: FormData,
): Promise<CsvActionResult> {
  const session = await readSession();
  if (!session) {
    return {
      ok: false,
      error: { code: "no_session", message: "Your session has expired. Please sign in again." },
    };
  }

  const file = formData.get("file");
  if (!(file instanceof File) || file.size === 0) {
    return {
      ok: false,
      error: { code: "no_file", message: "Please select a CSV file to upload." },
    };
  }

  const result = await submitCsvImport(session, storeId, file);
  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath("/products/import");
  redirect(`/products/import/${result.data.id}`);
}

export async function cancelCsvImportAction(
  storeId: string,
  jobId: string,
): Promise<CsvActionResult> {
  const session = await readSession();
  if (!session) {
    return {
      ok: false,
      error: { code: "no_session", message: "Your session has expired. Please sign in again." },
    };
  }

  const result = await cancelCsvImport(session, storeId, jobId);
  if (!result.ok) {
    return { ok: false, error: result.error };
  }

  revalidatePath(`/products/import/${jobId}`);
  return { ok: true };
}

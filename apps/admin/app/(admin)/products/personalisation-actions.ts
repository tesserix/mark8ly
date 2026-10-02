"use server";

// apps/admin/app/(admin)/products/personalisation-actions.ts
//
// Server actions for the Personalisation section of the product form
// (#962). Separate from actions.ts because this section does not share
// the product form's submit: each change is its own request, applied
// immediately, like Media.
//
// Every action returns the FULL field after the write rather than just
// ok/error. The editor then renders server truth instead of its own
// optimistic guess — which matters here because the API applies defaults
// (max_images, a text field's max_length) that the merchant never typed,
// and an optimistic row would show them as blank until a reload.

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  addPersonalisationOption,
  createPersonalisationField,
  deletePersonalisationField,
  deletePersonalisationOption,
  updatePersonalisationField,
  updatePersonalisationOption,
  type CreatePersonalisationFieldInput,
  type MutationError,
  type PersonalisationField,
  type SessionHeaders,
  type UpdatePersonalisationFieldInput,
} from "@/lib/api/marketplace-api";

export type PersonalisationActionResult =
  | { ok: true; field: PersonalisationField | null }
  | { ok: false; error: MutationError };

const noSession: PersonalisationActionResult = {
  ok: false,
  error: { code: "no_session", message: "Session expired. Please sign in again." },
};

async function readSession(storeId: string): Promise<SessionHeaders | null> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const email = h.get("x-session-email") ?? "";
  if (!userId || !tenantId || !storeId) return null;
  return { userId, tenantId, email };
}

// The product page is server-rendered with the field list, so every
// mutation has to invalidate it or a hard navigation would show stale
// rows. The editor also holds the returned field, so the UI is correct
// before the revalidation lands.
function revalidateProduct(productId: string): void {
  revalidatePath(`/products/${productId}`);
}

export async function createPersonalisationFieldAction(
  storeId: string,
  productId: string,
  input: CreatePersonalisationFieldInput,
): Promise<PersonalisationActionResult> {
  const session = await readSession(storeId);
  if (!session) return noSession;

  const res = await createPersonalisationField(storeId, productId, input, session);
  if (!res.ok) return { ok: false, error: res.error };
  revalidateProduct(productId);
  return { ok: true, field: res.data };
}

export async function updatePersonalisationFieldAction(
  storeId: string,
  productId: string,
  fieldId: string,
  input: UpdatePersonalisationFieldInput,
): Promise<PersonalisationActionResult> {
  const session = await readSession(storeId);
  if (!session) return noSession;

  const res = await updatePersonalisationField(storeId, productId, fieldId, input, session);
  if (!res.ok) return { ok: false, error: res.error };
  revalidateProduct(productId);
  return { ok: true, field: res.data };
}

export async function deletePersonalisationFieldAction(
  storeId: string,
  productId: string,
  fieldId: string,
): Promise<PersonalisationActionResult> {
  const session = await readSession(storeId);
  if (!session) return noSession;

  const res = await deletePersonalisationField(storeId, productId, fieldId, session);
  if (!res.ok) return { ok: false, error: res.error };
  revalidateProduct(productId);
  // Nothing to render back: the field is gone.
  return { ok: true, field: null };
}

export async function addPersonalisationOptionAction(
  storeId: string,
  productId: string,
  fieldId: string,
  input: { value: string; label: string; price_delta?: string },
): Promise<PersonalisationActionResult> {
  const session = await readSession(storeId);
  if (!session) return noSession;

  const res = await addPersonalisationOption(storeId, productId, fieldId, input, session);
  if (!res.ok) return { ok: false, error: res.error };
  revalidateProduct(productId);
  return { ok: true, field: res.data };
}

export async function updatePersonalisationOptionAction(
  storeId: string,
  productId: string,
  fieldId: string,
  optionId: string,
  input: { value?: string; label?: string; price_delta?: string },
): Promise<PersonalisationActionResult> {
  const session = await readSession(storeId);
  if (!session) return noSession;

  const res = await updatePersonalisationOption(
    storeId, productId, fieldId, optionId, input, session,
  );
  if (!res.ok) return { ok: false, error: res.error };
  revalidateProduct(productId);
  return { ok: true, field: res.data };
}

export async function deletePersonalisationOptionAction(
  storeId: string,
  productId: string,
  fieldId: string,
  optionId: string,
): Promise<PersonalisationActionResult> {
  const session = await readSession(storeId);
  if (!session) return noSession;

  const res = await deletePersonalisationOption(
    storeId, productId, fieldId, optionId, session,
  );
  if (!res.ok) return { ok: false, error: res.error };
  revalidateProduct(productId);
  return { ok: true, field: res.data };
}

"use server";

import { cookies } from "next/headers";
import { getMyProviders, type LinkedProvider } from "@/lib/auth/auth-bff";

export async function refreshProvidersAction(): Promise<LinkedProvider[]> {
  const c = await cookies();
  const cookieHeader = c
    .getAll()
    .map((x) => `${x.name}=${x.value}`)
    .join("; ");
  const result = await getMyProviders(cookieHeader);
  return result.providers;
}

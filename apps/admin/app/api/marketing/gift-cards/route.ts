import { NextRequest, NextResponse } from "next/server";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { issueGiftCard } from "@/lib/api/marketplace-api";

export async function POST(request: NextRequest) {
  const session = await getServerSessionContext();
  if (!session.currentStore) {
    return NextResponse.json({ message: "No store selected" }, { status: 400 });
  }

  const body = await request.json();

  try {
    const result = await issueGiftCard(
      session.currentStore.id,
      body,
      { userId: session.userId, tenantId: session.tenantId },
    );
    return NextResponse.json(result);
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return NextResponse.json({ message }, { status: 500 });
  }
}

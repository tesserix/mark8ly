import { NextRequest, NextResponse } from "next/server";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { deleteCampaign } from "@/lib/api/campaigns-api";

export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const session = await getServerSessionContext();
  if (!session.currentStore) {
    return NextResponse.json({ message: "No store selected" }, { status: 400 });
  }

  const { id } = await params;

  try {
    const ok = await deleteCampaign(
      session.currentStore.id,
      id,
      { userId: session.userId, tenantId: session.tenantId },
    );
    if (!ok) {
      return NextResponse.json(
        { message: "Failed to delete campaign" },
        { status: 502 },
      );
    }
    return NextResponse.json({ ok: true });
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return NextResponse.json({ message }, { status: 500 });
  }
}

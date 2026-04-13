import { NextRequest, NextResponse } from "next/server";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { sendCampaign } from "@/lib/api/campaigns-api";

export async function POST(
  _request: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const session = await getServerSessionContext();
  if (!session.currentStore) {
    return NextResponse.json({ message: "No store selected" }, { status: 400 });
  }

  const { id } = await params;

  try {
    const result = await sendCampaign(
      session.currentStore.id,
      id,
      { userId: session.userId, tenantId: session.tenantId },
    );
    if (!result) {
      return NextResponse.json(
        { message: "Failed to send campaign" },
        { status: 502 },
      );
    }
    return NextResponse.json(result);
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return NextResponse.json({ message }, { status: 500 });
  }
}

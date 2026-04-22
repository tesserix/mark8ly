import { NextRequest, NextResponse } from "next/server";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { updateSegment } from "@/lib/api/campaigns-api";

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const session = await getServerSessionContext();
  if (!session.currentStore) {
    return NextResponse.json({ message: "No store selected" }, { status: 400 });
  }

  const body = await request.json();

  try {
    const result = await updateSegment(
      session.currentStore.id,
      id,
      body,
      { userId: session.userId, tenantId: session.tenantId },
    );
    if (!result) {
      return NextResponse.json(
        { message: "Failed to update segment" },
        { status: 502 },
      );
    }
    return NextResponse.json(result);
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return NextResponse.json({ message }, { status: 500 });
  }
}

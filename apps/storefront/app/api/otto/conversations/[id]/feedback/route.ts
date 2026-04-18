import { forwardToOtto } from "../../../_proxy";

// POST /api/otto/conversations/[id]/feedback — customer submits the
// post-case survey (call rating, query resolved, staff rating,
// optional comments). Only accepted when the conversation is closed.
export async function POST(
  req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const body = await req.text();
  return forwardToOtto(
    `/api/v1/storefront/otto/conversations/${encodeURIComponent(id)}/feedback`,
    { method: "POST", body },
  );
}

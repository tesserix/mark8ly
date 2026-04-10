import Link from "next/link";

interface CampaignsListEmptyProps {
  variant: "no-campaigns" | "no-store";
}

export function CampaignsListEmpty({ variant }: CampaignsListEmptyProps) {
  if (variant === "no-store") {
    return (
      <div className="flex flex-col items-start gap-2 rounded-md border border-ink-200 bg-white px-6 py-12">
        <p className="text-sm text-ink-500">
          Create a store first to start managing campaigns.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-start gap-4 rounded-md border border-ink-200 bg-white px-6 py-12">
      <h2 className="font-serif text-lg font-semibold text-ink-900">
        No campaigns yet
      </h2>
      <p className="text-sm text-ink-500">
        Create your first campaign to reach your customers with targeted
        messages.
      </p>
      <Link
        href="/marketing/campaigns/new"
        className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800"
      >
        Create campaign
      </Link>
    </div>
  );
}

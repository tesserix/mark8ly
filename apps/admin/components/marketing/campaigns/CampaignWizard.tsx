"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";
import type {
  AdminSegment,
  CreateCampaignBody,
} from "@/lib/api/campaigns-api";
import { CampaignEditor } from "./CampaignEditor";

// ---------- Templates ----------

interface CampaignTemplate {
  id: string;
  name: string;
  subject: string;
  content: string;
}

const CAMPAIGN_TEMPLATES: CampaignTemplate[] = [
  {
    id: "announce-sale",
    name: "Announce a sale",
    subject: "Big savings await — our biggest sale starts now",
    content: `<h2>Our Biggest Sale of the Season</h2>
<p>For a limited time, enjoy exclusive discounts across our entire collection. Whether you've had your eye on something special or are looking for something new, now is the perfect time.</p>
<p><strong>Shop now and save before it's gone.</strong></p>`,
  },
  {
    id: "re-engage",
    name: "Re-engage inactive customers",
    subject: "We miss you — here's something special",
    content: `<h2>It's Been a While</h2>
<p>We noticed you haven't visited in a while, and we wanted to reach out. Our collection has grown since your last visit, and we think you'll love what's new.</p>
<p><strong>Come back and see what you've been missing.</strong></p>`,
  },
  {
    id: "welcome",
    name: "Welcome new subscribers",
    subject: "Welcome — glad you're here",
    content: `<h2>Welcome to the Family</h2>
<p>Thank you for joining us. We're a small team passionate about quality, and we're glad to have you along for the journey.</p>
<p>As a welcome, here's a look at some of our most popular items to get you started.</p>`,
  },
];

// ---------- Props ----------

interface CampaignWizardProps {
  storeId: string;
  segments: AdminSegment[];
}

type ScheduleMode = "now" | "later";

// ---------- Component ----------

export function CampaignWizard({
  storeId,
  segments,
}: CampaignWizardProps) {
  const router = useRouter();
  const [step, setStep] = useState(1);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showSendDialog, setShowSendDialog] = useState(false);

  // Step 1 — Audience
  const [name, setName] = useState("");
  const [segmentId, setSegmentId] = useState<string>("");

  // Step 2 — Content
  const [subject, setSubject] = useState("");
  const [content, setContent] = useState("");
  const [selectedTemplate, setSelectedTemplate] = useState<string | null>(null);

  // Step 3 — Schedule & Storefront
  const [scheduleMode, setScheduleMode] = useState<ScheduleMode>("now");
  const [scheduledAt, setScheduledAt] = useState("");
  const [showOnStorefront, setShowOnStorefront] = useState(false);
  const [storefrontLabel, setStorefrontLabel] = useState("");

  const selectedSegment = segments.find((s) => s.id === segmentId);

  function applyTemplate(template: CampaignTemplate) {
    setSelectedTemplate(template.id);
    setSubject(template.subject);
    setContent(template.content);
  }

  const canProceedStep1 = name.trim().length > 0;
  const canProceedStep2 = subject.trim().length > 0 && content.trim().length > 0;
  const canSubmit =
    scheduleMode === "now" || (scheduleMode === "later" && scheduledAt.length > 0);

  const handleSubmit = useCallback(
    async (mode: "draft" | "send" | "schedule") => {
      setSubmitting(true);
      setError(null);

      const body: CreateCampaignBody = {
        name: name.trim(),
        type: "email",
        subject: subject.trim(),
        content: content.trim(),
        ...(segmentId ? { segment_id: segmentId } : {}),
        show_on_storefront: showOnStorefront,
        ...(showOnStorefront && storefrontLabel.trim()
          ? { storefront_label: storefrontLabel.trim() }
          : {}),
      };

      try {
        // Create via same-origin proxy (server attaches auth headers).
        const createRes = await fetch("/api/marketing/campaigns", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        if (!createRes.ok) {
          const err = await createRes.json().catch(() => null);
          setError(err?.message ?? "Failed to create campaign. Please try again.");
          setSubmitting(false);
          return;
        }

        const created = await createRes.json();
        const campaignId = created.data.id;

        if (mode === "send") {
          const sendRes = await fetch(
            `/api/marketing/campaigns/${campaignId}/send`,
            { method: "POST" },
          );
          if (!sendRes.ok) {
            setError(
              "Campaign created but failed to send. You can send it from the detail page.",
            );
            setSubmitting(false);
            return;
          }
        } else if (mode === "schedule") {
          const schedRes = await fetch(
            `/api/marketing/campaigns/${campaignId}/schedule`,
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ scheduled_at: scheduledAt }),
            },
          );
          if (!schedRes.ok) {
            setError(
              "Campaign created but failed to schedule. You can schedule it from the detail page.",
            );
            setSubmitting(false);
            return;
          }
        }

        router.push("/marketing/campaigns");
        router.refresh();
      } catch {
        setError("An unexpected error occurred. Please try again.");
        setSubmitting(false);
      }
    },
    [name, subject, content, segmentId, scheduledAt, showOnStorefront, storefrontLabel, router],
  );

  return (
    <div className="space-y-8">
      {/* Progress indicator */}
      <div className="flex items-center gap-2 text-sm text-ink-500">
        {[1, 2, 3].map((s) => {
          const stepName = s === 1 ? "Audience" : s === 2 ? "Content" : "Schedule";
          return (
          <div key={s} className="flex items-center gap-2">
            <span
              aria-label={`Step ${s} of 3: ${stepName}`}
              className={`flex h-6 w-6 items-center justify-center rounded-full text-xs font-medium ${
                s === step
                  ? "bg-ink-900 text-paper-200"
                  : s < step
                    ? "bg-moss-700 text-white"
                    : "bg-ink-100 text-ink-500"
              }`}
            >
              {s}
            </span>
            <span
              className={
                s === step ? "font-medium text-ink-900" : "text-ink-500"
              }
            >
              {stepName}
            </span>
            {s < 3 && (
              <span className="mx-2 h-px w-8 bg-ink-200" aria-hidden />
            )}
          </div>
          );
        })}
      </div>

      <hr className="border-ink-200" />

      {/* Step 1 — Audience */}
      {step === 1 && (
        <div className="space-y-6">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-ink-900">
            Choose your audience
          </h2>

          <div className="space-y-4">
            <label className="block">
              <span className="text-sm font-medium text-ink-700">
                Campaign name
              </span>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g. Summer Sale Announcement"
                className="mt-1 block w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-500 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
              />
            </label>

            <div className="space-y-1">
              <label
                htmlFor="campaign-segment"
                className="text-sm font-medium text-ink-700"
              >
                Segment
              </label>
              <Select
                value={segmentId || "__all__"}
                onValueChange={(value) =>
                  setSegmentId(value === "__all__" ? "" : value)
                }
              >
                <SelectTrigger id="campaign-segment" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__all__">All customers</SelectItem>
                  {segments.map((seg) => (
                    <SelectItem key={seg.id} value={seg.id}>
                      {seg.name} ({seg.member_count.toLocaleString()} members)
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {selectedSegment && (
                <p className="text-xs text-ink-500">
                  Estimated {selectedSegment.member_count.toLocaleString()}{" "}
                  recipients
                </p>
              )}
            </div>
          </div>

          <div className="flex justify-end">
            <button
              onClick={() => setStep(2)}
              disabled={!canProceedStep1}
              className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
            >
              Next
            </button>
          </div>
        </div>
      )}

      {/* Step 2 — Content */}
      {step === 2 && (
        <div className="space-y-6">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-ink-900">
            Create your content
          </h2>

          {/* Starter templates */}
          <div>
            <p className="mb-3 text-sm font-medium text-ink-700">
              Start with a template
            </p>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              {CAMPAIGN_TEMPLATES.map((t) => (
                <button
                  key={t.id}
                  onClick={() => applyTemplate(t)}
                  className={`rounded-md border p-4 text-left transition ${
                    selectedTemplate === t.id
                      ? "border-moss-700 bg-moss-50"
                      : "border-ink-200 bg-white hover:border-ink-300"
                  }`}
                >
                  <p className="text-sm font-medium text-ink-900">{t.name}</p>
                  <p className="mt-1 line-clamp-2 text-xs text-ink-500">
                    {t.subject}
                  </p>
                </button>
              ))}
            </div>
          </div>

          <hr className="border-ink-200" />

          <div className="space-y-4">
            <label className="block">
              <span className="text-sm font-medium text-ink-700">
                Subject line
              </span>
              <input
                type="text"
                value={subject}
                onChange={(e) => {
                  setSubject(e.target.value);
                  setSelectedTemplate(null);
                }}
                placeholder="e.g. Big savings await"
                className="mt-1 block w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-500 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
              />
            </label>

            <div className="space-y-1">
              <span className="text-sm font-medium text-ink-700">Content</span>
              <CampaignEditor
                content={content}
                onChange={(html) => {
                  setContent(html);
                  setSelectedTemplate(null);
                }}
              />
            </div>
          </div>

          <div className="flex justify-between">
            <button
              onClick={() => setStep(1)}
              className="inline-flex items-center gap-2 rounded-md border border-ink-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-ink-50"
            >
              Back
            </button>
            <button
              onClick={() => setStep(3)}
              disabled={!canProceedStep2}
              className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
            >
              Next
            </button>
          </div>
        </div>
      )}

      {/* Step 3 — Schedule */}
      {step === 3 && (
        <div className="space-y-6">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-ink-900">
            Schedule delivery
          </h2>

          {/* Review summary */}
          <div className="rounded-md border border-ink-200 bg-white p-4 space-y-2">
            <p className="text-sm text-ink-500">
              <span className="font-medium text-ink-700">Campaign:</span>{" "}
              {name}
            </p>
            <p className="text-sm text-ink-500">
              <span className="font-medium text-ink-700">Audience:</span>{" "}
              {selectedSegment ? selectedSegment.name : "All customers"}
            </p>
            <p className="text-sm text-ink-500">
              <span className="font-medium text-ink-700">Subject:</span>{" "}
              {subject}
            </p>
          </div>

          {/* Storefront display */}
          <div className="space-y-3">
            <label className="flex items-center gap-3">
              <input
                type="checkbox"
                checked={showOnStorefront}
                onChange={(e) => setShowOnStorefront(e.target.checked)}
                className="size-4 rounded border-ink-300 accent-moss-700"
              />
              <span className="text-sm text-ink-900">
                Display a promotion bar on your storefront while this campaign
                is active
              </span>
            </label>

            {showOnStorefront && (
              <label className="block max-w-md">
                <span className="text-sm font-medium text-ink-700">
                  Promotion bar text
                </span>
                <input
                  type="text"
                  value={storefrontLabel}
                  onChange={(e) => setStorefrontLabel(e.target.value)}
                  placeholder="e.g. Summer sale — 20% off everything"
                  maxLength={120}
                  className="mt-1 block w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-500 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
                />
                <p className="mt-1 text-xs text-ink-500">
                  If the campaign has a linked coupon, the code will appear
                  automatically with a copy button.
                </p>
              </label>
            )}
          </div>

          <hr className="border-ink-200" />

          {/* Schedule options */}
          <fieldset className="space-y-3">
            <legend className="text-sm font-medium text-ink-700">
              When to send
            </legend>
            <label className="flex items-center gap-3">
              <input
                type="radio"
                name="schedule"
                value="now"
                checked={scheduleMode === "now"}
                onChange={() => setScheduleMode("now")}
                className="accent-moss-700"
              />
              <span className="text-sm text-ink-900">Send now</span>
            </label>
            <label className="flex items-center gap-3">
              <input
                type="radio"
                name="schedule"
                value="later"
                checked={scheduleMode === "later"}
                onChange={() => setScheduleMode("later")}
                className="accent-moss-700"
              />
              <span className="text-sm text-ink-900">Schedule for later</span>
            </label>

            {scheduleMode === "later" && (
              <label className="mt-2 block max-w-xs">
                <span className="text-sm font-medium text-ink-700">
                  Send date and time
                </span>
                <input
                  type="datetime-local"
                  value={scheduledAt}
                  onChange={(e) => setScheduledAt(e.target.value)}
                  className="mt-1 block w-full rounded-md border border-ink-200 bg-white px-3 py-2.5 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
                />
              </label>
            )}
          </fieldset>

          {error && (
            <p className="rounded-md bg-signal-50 px-3 py-2 text-sm text-signal-700">
              {error}
            </p>
          )}

          <div className="flex items-center justify-between">
            <button
              onClick={() => setStep(2)}
              className="inline-flex items-center gap-2 rounded-md border border-ink-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-ink-50"
            >
              Back
            </button>

            <div className="flex gap-3">
              <button
                onClick={() => handleSubmit("draft")}
                disabled={submitting}
                className="inline-flex items-center gap-2 rounded-md border border-ink-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-ink-50 disabled:opacity-50"
              >
                Save as draft
              </button>

              {scheduleMode === "later" ? (
                <button
                  onClick={() => handleSubmit("schedule")}
                  disabled={submitting || !canSubmit}
                  className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
                >
                  {submitting ? "Scheduling..." : "Schedule"}
                </button>
              ) : (
                <button
                  onClick={() => setShowSendDialog(true)}
                  disabled={submitting}
                  className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
                >
                  Send now
                </button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Send confirmation dialog */}
      <Dialog open={showSendDialog} onOpenChange={setShowSendDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Send campaign?</DialogTitle>
            <DialogDescription>
              This will send &ldquo;{name}&rdquo; to{" "}
              {selectedSegment
                ? `${selectedSegment.member_count.toLocaleString()} recipients`
                : "all customers"}
              . This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild>
              <button className="inline-flex items-center gap-2 rounded-md border border-ink-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-ink-50">
                Cancel
              </button>
            </DialogClose>
            <button
              onClick={() => {
                setShowSendDialog(false);
                handleSubmit("send");
              }}
              disabled={submitting}
              className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
            >
              {submitting ? "Sending..." : "Send now"}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

"use client";

import { useActionState, useState } from "react";
import { Button, Label } from "@tesserix/web";
import { saveTestimonial } from "@/app/(admin)/settings/testimonial/actions";
import type { MerchantTestimonial } from "@/lib/api/testimonials";

export function TestimonialForm({
  initial,
  storeName,
}: {
  initial: MerchantTestimonial | null;
  storeName: string;
}) {
  const [state, action, pending] = useActionState(saveTestimonial, {});
  const [quote, setQuote] = useState(initial?.quote ?? "");
  const [rating, setRating] = useState(String(initial?.rating ?? ""));
  const [showName, setShowName] = useState(Boolean(initial?.display_name));
  const [displayName, setDisplayName] = useState(initial?.display_name ?? "");
  const [consent, setConsent] = useState(false);
  const published = state.published ?? initial?.published ?? false;
  return (
    <form action={action} className="max-w-2xl space-y-6">
      <p className="text-sm text-foreground-secondary">
        Share your experience of using Mark8ly for {storeName}. Choose the
        rating that reflects your experience. Your review may appear on our
        homepage. You can edit it or save it privately at any time.
      </p>
      <p className="text-sm font-medium">
        Status: {published ? "Published" : "Private draft"}
      </p>
      <p className="text-sm text-foreground-secondary">
        Public attribution shows your store name and store role. Your name is
        shown only if you choose to add it below.
      </p>
      <div className="space-y-2">
        <Label htmlFor="testimonial-rating">Your rating</Label>
        <select
          id="testimonial-rating"
          name="rating"
          value={rating}
          required
          disabled={pending}
          className="block w-full rounded-md border border-border-subtle bg-background p-3 text-foreground"
          onChange={(e) => {
            setRating(e.target.value);
            setConsent(false);
          }}
        >
          <option value="">Choose a rating</option>
          {[1, 2, 3, 4, 5].map((n) => (
            <option key={n} value={n}>
              {n} {n === 1 ? "star" : "stars"}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-2">
        <Label htmlFor="testimonial-quote">Your review</Label>
        <textarea
          id="testimonial-quote"
          name="quote"
          value={quote}
          maxLength={1000}
          rows={6}
          required
          disabled={pending}
          className="block w-full rounded-md border border-border-subtle bg-background p-3 text-foreground"
          onChange={(e) => {
            setQuote(e.target.value);
            setConsent(false);
          }}
        />
        <p className="text-xs text-foreground-secondary">
          Please check that these are your own words and reflect your
          experience.
        </p>
      </div>
      <div className="space-y-3">
        <label className="flex items-start gap-3 text-sm">
          <input
            type="checkbox"
            name="show_name"
            checked={showName}
            disabled={pending}
            className="mt-1"
            onChange={(e) => {
              setShowName(e.target.checked);
              setConsent(false);
            }}
          />
          <span>Show my name on the review</span>
        </label>
        {showName ? (
          <div className="space-y-2">
            <Label htmlFor="testimonial-name">Your name</Label>
            <input
              id="testimonial-name"
              name="display_name"
              value={displayName}
              maxLength={80}
              required
              disabled={pending}
              autoComplete="name"
              className="block w-full rounded-md border border-border-subtle bg-background p-3 text-foreground"
              onChange={(e) => {
                setDisplayName(e.target.value);
                setConsent(false);
              }}
            />
            <p className="text-xs text-foreground-secondary">
              Shown next to your store role, for example &ldquo;Store owner ·
              Sam&rdquo;. Untick to hide it again.
            </p>
          </div>
        ) : null}
      </div>
      <label className="flex items-start gap-3 text-sm">
        <input
          type="checkbox"
          name="consent"
          checked={consent}
          disabled={pending}
          className="mt-1"
          onChange={(e) => setConsent(e.target.checked)}
        />
        <span>
          I confirm this is my honest review and agree to publish my store name,
          store role,{showName ? " name," : ""} rating and words on mark8ly.com.
        </span>
      </label>
      {state.message ? (
        <p
          role={state.ok ? "status" : "alert"}
          className={state.ok ? "text-sm" : "text-sm text-danger"}
        >
          {state.message}
        </p>
      ) : null}
      <div className="flex flex-wrap gap-3">
        <Button
          type="submit"
          name="intent"
          value="publish"
          disabled={pending || !consent}
        >
          {pending ? "Saving…" : "Publish review"}
        </Button>
        <Button
          type="submit"
          name="intent"
          value="draft"
          variant="outline"
          disabled={pending}
        >
          {published ? "Withdraw and save privately" : "Save privately"}
        </Button>
      </div>
    </form>
  );
}

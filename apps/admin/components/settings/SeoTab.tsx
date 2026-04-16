"use client";

import type { StoreBranding } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "./BrandingSettingsClient";

interface SeoTabProps {
  form: StoreBranding;
  patch: (updates: Partial<StoreBranding>) => void;
  editable: boolean;
}

type AIPolicy = StoreBranding["seo_ai_policy"];

const AI_POLICY_OPTIONS: Array<{
  value: AIPolicy;
  label: string;
  description: string;
}> = [
  {
    value: "allow",
    label: "Allow all AI crawlers",
    description: "GPTBot, Google-Extended, CCBot, ClaudeBot, and others can index and train on your content.",
  },
  {
    value: "training-only-denied",
    label: "Indexing OK, no AI training",
    description: "AI assistants can cite and link your store, but your content won't be used to train models.",
  },
  {
    value: "deny",
    label: "Block all AI crawlers",
    description: "Blocks GPTBot, CCBot, Google-Extended, Anthropic, ByteSpider, PerplexityBot. Search engines still index normally.",
  },
];

export function SeoTab({ form, patch, editable }: SeoTabProps) {
  return (
    <div className="space-y-10">
      {/* ── Core meta ────────────────────────────────────────── */}
      <section className="space-y-6">
        <div className="space-y-1">
          <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
            Core meta
          </h2>
          <p className="text-sm text-foreground-secondary">
            Used for page titles and descriptions when a specific page doesn&apos;t override them. Also emitted as OpenGraph + Twitter card fallbacks.
          </p>
        </div>

        <div className="space-y-1.5">
          <FieldLabel htmlFor="seo_title_template">Title template</FieldLabel>
          <TextInput
            id="seo_title_template"
            value={form.seo_title_template ?? ""}
            onChange={(v) => patch({ seo_title_template: v || null })}
            placeholder="%s — India Store"
            disabled={!editable}
            maxLength={200}
          />
          <p className="text-xs text-foreground-secondary">
            <code>%s</code> is replaced by the page title. Example: <code>&quot;Shirts — India Store&quot;</code>.
          </p>
        </div>

        <div className="space-y-1.5">
          <FieldLabel htmlFor="seo_default_description">Default description</FieldLabel>
          <textarea
            id="seo_default_description"
            value={form.seo_default_description ?? ""}
            onChange={(e) => patch({ seo_default_description: e.target.value || null })}
            disabled={!editable}
            rows={3}
            maxLength={300}
            placeholder="Handwoven textiles and quiet objects from small ateliers across India."
            className="w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
          />
          <p className="text-xs text-foreground-secondary">
            150–160 characters is the sweet spot for Google snippets.
          </p>
        </div>

        <div className="space-y-1.5">
          <FieldLabel htmlFor="seo_og_image_url">Social share image URL</FieldLabel>
          <TextInput
            id="seo_og_image_url"
            value={form.seo_og_image_url ?? ""}
            onChange={(v) => patch({ seo_og_image_url: v || null })}
            placeholder="https://cdn.example.com/og-image.jpg"
            disabled={!editable}
            maxLength={2048}
          />
          <p className="text-xs text-foreground-secondary">
            Recommended 1200×630. Used for OpenGraph and Twitter card images.
          </p>
        </div>

        <div className="space-y-1.5">
          <FieldLabel htmlFor="seo_twitter_handle">Twitter / X handle</FieldLabel>
          <TextInput
            id="seo_twitter_handle"
            value={form.seo_twitter_handle ?? ""}
            onChange={(v) => patch({ seo_twitter_handle: v || null })}
            placeholder="@indiastore"
            disabled={!editable}
            maxLength={100}
          />
        </div>
      </section>

      {/* ── Search engine verification ──────────────────────── */}
      <section className="space-y-6 border-t border-border-subtle pt-8">
        <div className="space-y-1">
          <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
            Search engine verification
          </h2>
          <p className="text-sm text-foreground-secondary">
            Meta tag verification codes from Google Search Console and Bing Webmaster Tools. Paste just the content value — we emit the full <code>&lt;meta&gt;</code> tag.
          </p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <FieldLabel htmlFor="seo_google_verification">Google verification</FieldLabel>
            <TextInput
              id="seo_google_verification"
              value={form.seo_google_verification ?? ""}
              onChange={(v) => patch({ seo_google_verification: v || null })}
              placeholder="abcdef1234567890…"
              disabled={!editable}
              maxLength={200}
            />
          </div>
          <div className="space-y-1.5">
            <FieldLabel htmlFor="seo_bing_verification">Bing verification</FieldLabel>
            <TextInput
              id="seo_bing_verification"
              value={form.seo_bing_verification ?? ""}
              onChange={(v) => patch({ seo_bing_verification: v || null })}
              placeholder="ABCD1234…"
              disabled={!editable}
              maxLength={200}
            />
          </div>
        </div>
      </section>

      {/* ── Structured data ─────────────────────────────────── */}
      <section className="space-y-6 border-t border-border-subtle pt-8">
        <div className="space-y-1">
          <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
            Structured data (JSON-LD)
          </h2>
          <p className="text-sm text-foreground-secondary">
            Schema.org JSON-LD emitted site-wide. Typically an <code>Organization</code> or <code>Store</code> object. Helps search engines and LLMs understand your brand.
          </p>
        </div>

        <div className="space-y-1.5">
          <FieldLabel htmlFor="seo_json_ld">JSON-LD</FieldLabel>
          <textarea
            id="seo_json_ld"
            value={form.seo_json_ld ?? ""}
            onChange={(e) => patch({ seo_json_ld: e.target.value || null })}
            disabled={!editable}
            rows={12}
            spellCheck={false}
            placeholder={`{\n  "@context": "https://schema.org",\n  "@type": "Store",\n  "name": "India Store",\n  "url": "https://indiastore.com",\n  "logo": "https://cdn.example.com/logo.png",\n  "sameAs": [\n    "https://instagram.com/indiastore",\n    "https://twitter.com/indiastore"\n  ]\n}`}
            className="w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 py-2.5 font-mono text-xs leading-5 text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
          />
          <p className="text-xs text-foreground-secondary">
            Validate with{" "}
            <a href="https://validator.schema.org/" target="_blank" rel="noopener noreferrer" className="underline underline-offset-2">
              schema.org validator
            </a>
            . Invalid JSON is silently dropped at render time.
          </p>
        </div>
      </section>

      {/* ── AI SEO ──────────────────────────────────────────── */}
      <section className="space-y-6 border-t border-border-subtle pt-8">
        <div className="space-y-1">
          <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
            AI crawler policy
          </h2>
          <p className="text-sm text-foreground-secondary">
            Controls how AI assistants (ChatGPT, Claude, Perplexity, Gemini) and their training crawlers treat your store. Applied via <code>robots.txt</code> and HTTP headers.
          </p>
        </div>

        <div className="space-y-3">
          {AI_POLICY_OPTIONS.map((option) => {
            const active = form.seo_ai_policy === option.value;
            return (
              <button
                key={option.value}
                type="button"
                onClick={() => editable && patch({ seo_ai_policy: option.value })}
                disabled={!editable}
                className={`block w-full rounded-md border p-4 text-left transition-colors ${
                  active
                    ? "border-[color:var(--moss-700)] bg-[color:var(--moss-700)]/5"
                    : "border-border bg-[color:var(--background-elevated)] hover:border-border-strong"
                } disabled:cursor-not-allowed disabled:opacity-60`}
              >
                <div className="flex items-start gap-3">
                  <span
                    className={`mt-0.5 inline-block h-4 w-4 rounded-full border-2 ${
                      active
                        ? "border-[color:var(--moss-700)] bg-[color:var(--moss-700)]"
                        : "border-border"
                    }`}
                  />
                  <div>
                    <p className="text-sm font-semibold text-foreground">{option.label}</p>
                    <p className="mt-1 text-xs leading-5 text-foreground-secondary">{option.description}</p>
                  </div>
                </div>
              </button>
            );
          })}
        </div>

        <div className="space-y-1.5 border-t border-border-subtle pt-6">
          <FieldLabel htmlFor="seo_llms_txt">Custom llms.txt</FieldLabel>
          <textarea
            id="seo_llms_txt"
            value={form.seo_llms_txt ?? ""}
            onChange={(e) => patch({ seo_llms_txt: e.target.value || null })}
            disabled={!editable}
            rows={10}
            spellCheck={false}
            placeholder={`# India Store\n\n> Handwoven textiles and quiet objects sourced from small ateliers across India.\n\n## Shop\n- [All products](/products): Browse our full catalog\n- [Collections](/categories): Curated collections\n\n## About\n- [Our story](/pages/about): How we started and what we value`}
            className="w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 py-2.5 font-mono text-xs leading-5 text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
          />
          <p className="text-xs text-foreground-secondary">
            Emerging standard (<a href="https://llmstxt.org/" target="_blank" rel="noopener noreferrer" className="underline underline-offset-2">llmstxt.org</a>). Plain-text summary of your site for LLM agents, served at <code>/llms.txt</code>. If empty, we auto-generate from your store name + description.
          </p>
        </div>
      </section>
    </div>
  );
}

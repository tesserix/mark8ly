"use client";

// apps/admin/components/settings/PageEditor.tsx
//
// Client-side editor for a single static page (Title, Slug, Body, SEO, Published).
// Live markdown preview rendered in the right column on lg+.
//
// Form convention: react-hook-form + zod + @hookform/resolvers/zod, matching
// the pattern established in GeneralSettingsForm.tsx.

import { useEffect, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import {
  Button,
  Input,
  Label,
  Switch,
  Textarea,
  AlertDialog,
} from "@tesserix/web";
import type { AdminPage } from "@/lib/api/marketplace-api";
import {
  updatePageAction,
  deletePageAction,
} from "@/app/(admin)/settings/pages/actions";
import { useToast } from "@/components/feedback/Toaster";
import { Markdown } from "@/lib/markdown";
import { PageBodyEditor } from "./PageBodyEditor";

// ─── Slug pattern ─────────────────────────────────────────────────────────────

const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/;

// ─── Zod schema ───────────────────────────────────────────────────────────────

const schema = z.object({
  title: z.string().min(1, "Title is required").max(200),
  slug: z
    .string()
    .min(3, "Slug must be at least 3 characters")
    .max(63)
    .regex(SLUG_PATTERN, "Lowercase letters, numbers, and hyphens only"),
  body: z.string(),
  seo_title: z.string().max(200).optional().or(z.literal("")),
  seo_description: z.string().max(300).optional().or(z.literal("")),
  published: z.boolean(),
});

type FormValues = z.infer<typeof schema>;

// ─── Helpers ──────────────────────────────────────────────────────────────────

function slugify(input: string): string {
  return input
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9\s-]/g, "")
    .replace(/\s+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

// ─── Component ────────────────────────────────────────────────────────────────

interface PageEditorProps {
  page: AdminPage;
  canManage: boolean;
}

export function PageEditor({ page, canManage }: PageEditorProps) {
  const router = useRouter();
  const { toast } = useToast();
  const [pending, startTransition] = useTransition();
  const [deleteOpen, setDeleteOpen] = useState(false);

  // Track whether the user has manually edited the slug so we stop
  // auto-deriving it from the title.
  const [slugTouched, setSlugTouched] = useState(
    page.slug !== slugify(page.title),
  );

  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors, isDirty },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      title: page.title,
      slug: page.slug,
      body: page.body ?? "",
      seo_title: page.seo_title ?? "",
      seo_description: page.seo_description ?? "",
      published: page.published,
    },
  });

  // Auto-derive slug from title while user hasn't touched the slug field.
  const title = watch("title");
  useEffect(() => {
    if (!slugTouched) {
      setValue("slug", slugify(title), { shouldValidate: false });
    }
  }, [title, slugTouched, setValue]);

  const onSubmit = (values: FormValues) => {
    startTransition(async () => {
      const result = await updatePageAction(page.id, {
        title: values.title,
        slug: values.slug,
        body: values.body,
        seo_title: values.seo_title || null,
        seo_description: values.seo_description || null,
        published: values.published,
      });
      if (result.ok) {
        toast.success("Page saved");
        router.refresh();
      } else {
        toast.error("Save failed", result.message);
      }
    });
  };

  const onDeleteConfirm = () => {
    setDeleteOpen(false);
    startTransition(async () => {
      const result = await deletePageAction(page.id);
      if (result.ok) {
        toast.success("Page deleted");
        router.push("/settings/pages");
      } else {
        toast.error("Delete failed", result.message);
      }
    });
  };

  const body = watch("body");
  const publishedValue = watch("published");

  return (
    <>
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-10">
        <div className="grid gap-10 lg:grid-cols-[1fr_1fr]">
          {/* ── Left: form fields ─────────────────────────────────── */}
          <div className="space-y-6">
            {/* Title */}
            <div>
              <Label htmlFor="title">Title</Label>
              <Input
                id="title"
                {...register("title")}
                disabled={!canManage}
                className="mt-1"
              />
              {errors.title && (
                <p className="mt-1 text-sm text-danger">
                  {errors.title.message}
                </p>
              )}
            </div>

            {/* Slug */}
            <div>
              <Label htmlFor="slug">URL slug</Label>
              <div className="mt-1 flex items-center gap-1">
                <span className="shrink-0 text-sm text-muted-foreground">
                  /pages/
                </span>
                <Input
                  id="slug"
                  {...register("slug")}
                  onChange={(e) => {
                    setSlugTouched(true);
                    setValue("slug", e.target.value, { shouldValidate: true });
                  }}
                  disabled={!canManage}
                />
              </div>
              {errors.slug && (
                <p className="mt-1 text-sm text-danger">
                  {errors.slug.message}
                </p>
              )}
            </div>

            {/* Body */}
            <div>
              <Label htmlFor="body">Body</Label>
              <div className="mt-1">
                <PageBodyEditor
                  markdown={body}
                  onChange={(md) => setValue("body", md, { shouldDirty: true })}
                  editable={canManage}
                />
              </div>
              <p className="mt-1.5 text-xs text-muted-foreground">
                Format with the toolbar — content is stored as markdown.
              </p>
            </div>

            {/* SEO */}
            <div className="space-y-3">
              <Label>SEO</Label>
              <Input
                placeholder="SEO title (optional)"
                {...register("seo_title")}
                disabled={!canManage}
              />
              {errors.seo_title && (
                <p className="mt-1 text-sm text-danger">
                  {errors.seo_title.message}
                </p>
              )}
              <Textarea
                placeholder="SEO description (optional)"
                rows={3}
                {...register("seo_description")}
                disabled={!canManage}
              />
              {errors.seo_description && (
                <p className="mt-1 text-sm text-danger">
                  {errors.seo_description.message}
                </p>
              )}
            </div>

            {/* Published toggle */}
            <div className="flex items-center gap-3">
              <Switch
                id="published"
                checked={publishedValue}
                onCheckedChange={(v) =>
                  setValue("published", v, { shouldDirty: true })
                }
                disabled={!canManage}
              />
              <Label htmlFor="published">Published</Label>
            </div>
          </div>

          {/* ── Right: live preview ───────────────────────────────── */}
          <aside className="lg:sticky lg:top-6 lg:self-start">
            <div className="rounded-md border border-border bg-background-elevated px-6 py-8">
              <p className="text-xs uppercase tracking-wide text-muted-foreground">
                Preview
              </p>
              <h2 className="mt-4 font-serif text-3xl font-medium tracking-tight text-foreground">
                {watch("title") || "Untitled"}
              </h2>
              <Markdown className="prose mt-6 max-w-none prose-headings:font-serif prose-headings:text-foreground prose-a:text-moss-700">
                {body || "_Start writing to see the preview…_"}
              </Markdown>
            </div>
          </aside>
        </div>

        {/* ── Action bar ──────────────────────────────────────────── */}
        <div className="flex items-center justify-between border-t border-border pt-6">
          {canManage ? (
            <Button
              type="button"
              variant="destructive"
              onClick={() => setDeleteOpen(true)}
              disabled={pending}
            >
              Delete
            </Button>
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <Button
              type="button"
              variant="secondary"
              onClick={() => router.push("/settings/pages")}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={!canManage || pending || !isDirty}
            >
              {pending ? "Saving…" : "Save changes"}
            </Button>
          </div>
        </div>
      </form>

      {/* Delete confirmation dialog */}
      <AlertDialog
        isOpen={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        title="Delete page"
        message="Are you sure you want to delete this page? This cannot be undone."
        type="confirm"
        confirmLabel="Delete"
        cancelLabel="Cancel"
        onConfirm={onDeleteConfirm}
        onCancel={() => setDeleteOpen(false)}
      />
    </>
  );
}

"use client";

// apps/admin/components/products/ProductForm.tsx
//
// M7c: tab shell host. Owns FormProvider, the top-level form state,
// the options->variants derivation effect, and the existing submit/delete
// flow from M7b. Individual tabs live under ./form/.

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  useTransition,
} from "react";
import { FormProvider, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Trash2 } from "lucide-react";
import { AlertDialog } from "@tesserix/web";

import { StatusDot } from "@repo/ui/status-dot";

import {
  productFormSchema,
  type ProductFormValues,
} from "@/lib/validation/product-form";
import type {
  AdminProduct,
  AdminCategory,
  SessionHeaders,
} from "@/lib/api/marketplace-api";
import {
  createProductAction,
  updateProductAction,
  deleteProductAction,
} from "@/app/(admin)/products/actions";
import {
  generateVariants,
  type OptionDraft as GenOptionDraft,
} from "@/lib/products/generateVariants";
import { buildVariantKey, parseVariantKey } from "@/lib/products/variantKey";
import type { VariantDraft as MatrixVariantDraft } from "@/components/products/variants/VariantMatrixTable";

import { useToast } from "@/components/feedback/Toaster";
import { useUnsavedGuard } from "@/lib/hooks/useUnsavedGuard";
import type { Warehouse } from "@/lib/api/warehouses-api";
import { OptionsTab } from "./form/OptionsTab";
import { MediaTab } from "./form/MediaTab";
import { TaxSection } from "./form/TaxSection";
import { ProductSection } from "./form/ProductSection";
import { DetailsSection } from "./form/DetailsSection";
import { PricingSection } from "./form/PricingSection";
import { ShippingSection } from "./form/ShippingSection";
import { ProductRail } from "./form/ProductRail";
import { Field } from "./form/Field";
import { ProductCategoriesPicker } from "./ProductCategoriesPicker";

// RHF-side option shape (matches zod + OptionsEditor).
interface RhfOptionDraft {
  id?: string;
  name: string;
  values: Array<{ id?: string; value: string }>;
}

export interface ProductFormProps {
  mode: "create" | "edit";
  storeId: string;
  initialProduct?: AdminProduct;
  categories: AdminCategory[];
  currencyCode: string;
  /** ISO 3166-1 alpha-2 code of the store, drives the Tax section strategy. */
  storeCountryCode: string;
  canDelete: boolean;
  canArchive: boolean;
  session: SessionHeaders;
  storeSlug?: string;
  /**
   * The store's live warehouses (#177 PR 5e). Empty or one keeps the
   * single Stock field and the ordinary product save — a store with one
   * warehouse must see exactly what it saw before.
   */
  warehouses?: Warehouse[];
}

export function ProductForm({
  mode,
  storeId,
  initialProduct,
  categories,
  currencyCode,
  storeCountryCode,
  canDelete,
  session,
  storeSlug,
  warehouses = [],
}: ProductFormProps) {
  const router = useRouter();
  const { toast } = useToast();
  const [isPending, startTransition] = useTransition();
  const [rootError, setRootError] = useState<string | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [discardOpen, setDiscardOpen] = useState(false);
  const hasMultipleVariants = (initialProduct?.variants.length ?? 0) > 1;
  // Whether this product has real options, as opposed to variants that
  // simply exist. 8 of the-bondi-store's 12 products have the latter: rows
  // with prices and SKUs but no product_options at all. Calling those
  // "combinations" is wrong — there is nothing being combined — and the
  // consequence of adding an option to them is destructive, so both bits of
  // copy below adapt on this rather than on variant count alone.
  const namedOptions = (initialProduct?.options ?? []).filter(
    (o) => o.name.trim().length > 0,
  );
  const hasOptions = namedOptions.length > 0;
  const firstVariant = initialProduct?.variants[0];

  // variantId -> warehouseId -> quantity, from the product detail response
  // (#177 PR 6). Empty for a store with fewer than two warehouses, which is
  // exactly when neither tab shows a per-warehouse editor.
  const stockByVariant: Record<string, Record<string, number>> = {};
  for (const v of initialProduct?.variants ?? []) {
    if (v.inventory_by_location) stockByVariant[v.id] = v.inventory_by_location;
  }

  const defaults: ProductFormValues = {
    title: initialProduct?.title ?? "",
    handle: initialProduct?.handle ?? "",
    description: initialProduct?.description ?? "",
    status: initialProduct?.status ?? "draft",
    price: firstVariant?.price ?? "",
    inventoryQuantity: firstVariant
      ? String(firstVariant.inventory_quantity)
      : "0",
    alwaysInStock: firstVariant?.inventory_policy === "continue",
    sku: firstVariant?.sku ?? "",
    weightKg: firstVariant?.weight_grams
      ? String(firstVariant.weight_grams / 1000)
      : "",
    lengthCm: firstVariant?.length_cm ?? "",
    widthCm: firstVariant?.width_cm ?? "",
    heightCm: firstVariant?.height_cm ?? "",
    categoryIds: initialProduct?.categories.map((c) => c.id) ?? [],
    taxCode: initialProduct?.tax_code ?? "",
    taxRateOverride: initialProduct?.tax_rate_override ?? "",
    taxCategory: initialProduct?.tax_category ?? undefined,
    // Hydrate options/variants from the server payload so a refresh after
    // save doesn't show an empty Options/Variants tab. Without this the
    // form resets to [] and the derivation effect then blows away variants
    // on mount because no options → no matrix.
    options: (initialProduct?.options ?? []).map((o) => ({
      id: o.id,
      name: o.name,
      values: o.values.map((v) => ({ id: v.id, value: v.value })),
    })),
    variants: (initialProduct?.variants ?? []).map((v) => {
      const pairs = v.option_values.map((ov) => ({
        name: ov.option_name,
        value: ov.value,
      }));
      const variantMedia = (initialProduct?.media ?? []).find(
        (m) => m.variant_id === v.id,
      );
      return {
        id: v.id,
        // With no options there is nothing to compose a key from and
        // buildVariantKey returns "" for every variant alike. The key is
        // both the React list key and the identity handlePatch matches on,
        // so a shared "" made editing one row write into all of them. Fall
        // back to the variant's own id, which is unique and stable.
        key: pairs.length > 0 ? buildVariantKey(pairs) : `id:${v.id}`,
        price: v.price,
        sku: v.sku,
        stock: v.inventory_quantity,
        weight: (v.weight_grams ?? 0) / 1000,
        lengthCm: v.length_cm ? Number(v.length_cm) : undefined,
        widthCm: v.width_cm ? Number(v.width_cm) : undefined,
        heightCm: v.height_cm ? Number(v.height_cm) : undefined,
        variantImageId: variantMedia?.id ?? null,
        optionValues: v.option_values.map((ov) => ({
          optionName: ov.option_name,
          value: ov.value,
        })),
      };
    }),
    // Hydrate media from the server payload so a title/price-only edit
    // doesn't submit `media: []` and wipe the product's existing images.
    media: (initialProduct?.media ?? []).map((m) => ({
      id: m.id,
      url: m.url,
      alt: m.alt ?? "",
      position: m.position,
      variant_id: m.variant_id ?? null,
      storage_key: m.storage_key,
      gcs_path_original: "",
    })),
    removed_variant_ids: [],
  };

  const methods = useForm<ProductFormValues>({
    resolver: zodResolver(productFormSchema),
    defaultValues: defaults,
  });
  const { control, setValue, setError, clearErrors, getValues } = methods;

  // --- Derivation effect -------------------------------------------------
  // Rebuild the variants matrix whenever option *shape* changes. We read
  // current variants with getValues (no subscription) to avoid re-running
  // on every per-cell edit, which would cause an infinite loop.
  const options = useWatch({ control, name: "options" }) as
    | RhfOptionDraft[]
    | undefined;

  const accumulatedRemovedIdsRef = useRef<string[]>([]);

  const optionsSignature = useMemo(() => {
    if (!options) return "";
    return JSON.stringify(
      options.map((o) => ({
        id: o.id ?? null,
        name: o.name,
        values: (o.values ?? []).map((v) => ({
          id: v.id ?? null,
          value: v.value,
        })),
      })),
    );
  }, [options]);

  // Skips the FIRST run. On mount, options and variants are both hydrated
  // from the server and already agree with each other — there is nothing
  // to derive, and deriving anyway is destructive: generateVariants with
  // zero options returns every existing variant id as `removedIds`, which
  // is correct when a merchant deletes their last option and catastrophic
  // when the product simply never had options.
  //
  // 8 of the-bondi-store's 12 products are in that state (variants, no
  // option rows — the shape a seeded catalogue arrives in). Opening one
  // emptied the variants list and staged every variant for deletion;
  // pressing Save destroyed them. The form looked like it was showing an
  // empty state and was in fact holding a loaded gun.
  const derivationHasRun = useRef(false);

  useEffect(() => {
    if (!options) return;
    if (!derivationHasRun.current) {
      derivationHasRun.current = true;
      return;
    }

    // Adapt RHF options -> generateVariants OptionDraft (values: string[]).
    const adapted: GenOptionDraft[] = options
      .filter((o) => o.name.trim().length > 0)
      .map((o) => ({
        name: o.name,
        values: (o.values ?? [])
          .map((v) => v.value)
          .filter((v) => v.trim().length > 0),
      }))
      .filter((o) => o.values.length > 0);

    const currentVariants =
      (getValues("variants") as MatrixVariantDraft[] | undefined) ?? [];
    const baseDefaults = {
      price: (getValues("price") as string) ?? "0",
      sku: "",
      stock: 0,
      weight: 0,
    };

    try {
      const { variants, removedIds } = generateVariants(
        adapted,
        // generateVariants stores its own shape; optionValues is ignored by it.
        currentVariants as unknown as Parameters<typeof generateVariants>[1],
        baseDefaults,
      );

      // Enrich each variant with optionValues derived from its sorted key,
      // so VariantMatrixTable can display per-option columns.
      const enriched: MatrixVariantDraft[] = variants.map((v) => {
        const prior = currentVariants.find((p) => p.key === v.key);
        if (prior && prior.optionValues) {
          return { ...v, optionValues: prior.optionValues } as MatrixVariantDraft;
        }
        // Derive optionValues by parsing the canonical variant key format
        // (Name1=Value1|Name2=Value2, see lib/products/variantKey.ts).
        const optionValues = parseVariantKey(v.key)
          .map((p) => ({ optionName: p.name, value: p.value }))
          .filter((ov) => ov.optionName.length > 0);
        return { ...v, optionValues } as MatrixVariantDraft;
      });

      setValue("variants", enriched, {
        shouldDirty: true,
        shouldValidate: false,
      });
      if (removedIds.length > 0) {
        const nextAccum = [
          ...accumulatedRemovedIdsRef.current,
          ...removedIds,
        ];
        accumulatedRemovedIdsRef.current = nextAccum;
        setValue("removed_variant_ids", nextAccum, {
          shouldDirty: true,
          shouldValidate: false,
        });
      }
      clearErrors("variants");
    } catch (err) {
      setError("variants", {
        type: "cap",
        message: err instanceof Error ? err.message : "Too many variants",
      });
    }
    // Intentionally only depends on the signature.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [optionsSignature]);

  // --- Submit / delete ---------------------------------------------------
  const applyError = (err: {
    code: string;
    message: string;
    field?: string;
  }) => {
    if (err.field) {
      methods.setError(err.field as keyof ProductFormValues, {
        type: "server",
        message: err.message,
      });
    } else {
      setRootError(err.message);
    }
    toast.error(
      mode === "create" ? "Couldn't create product" : "Couldn't save changes",
      err.message,
    );
  };

  const onSubmit = (values: ProductFormValues) => {
    setRootError(null);
    startTransition(async () => {
      if (mode === "create") {
        const result = await createProductAction(storeId, currencyCode, values);
        if (!result.ok && result.error) {
          applyError(result.error);
        } else {
          accumulatedRemovedIdsRef.current = [];
          // createProductAction redirects on success, so this rarely
          // shows — included for parity if redirect is removed.
          toast.success("Product created");
        }
      } else if (initialProduct) {
        const result = await updateProductAction(
          storeId,
          initialProduct.id,
          currencyCode,
          values,
        );
        if (!result.ok && result.error) {
          applyError(result.error);
        } else {
          accumulatedRemovedIdsRef.current = [];
          toast.success("Changes saved");
          router.refresh();
        }
      }
    });
  };

  const handleDelete = () => {
    if (!initialProduct) return;
    setDeleteOpen(true);
  };

  const confirmDelete = () => {
    if (!initialProduct) return;
    setDeleteOpen(false);
    startTransition(async () => {
      const result = await deleteProductAction(storeId, initialProduct.id);
      if (!result.ok && result.error) {
        setRootError(result.error.message);
        toast.error("Couldn't delete product", result.error.message);
      } else {
        toast.success("Product deleted");
      }
    });
  };

  // Unsaved-changes guard — warn on tab close / reload / cross-site nav
  // when the form is dirty. In-app nav via Discard opens a confirm dialog
  // instead (see handleDiscard below) because App Router doesn't fire
  // beforeunload on client-side navigation.
  useUnsavedGuard(methods.formState.isDirty, isPending);

  const handleDiscard = (e: React.MouseEvent<HTMLAnchorElement>) => {
    if (methods.formState.isDirty && !isPending) {
      e.preventDefault();
      setDiscardOpen(true);
    }
  };

  const confirmDiscard = () => {
    setDiscardOpen(false);
    router.push("/products");
  };

  const onInvalid = (errors: Record<string, unknown>) => {
    // RHF blocked submit due to validation. Surface a clear toast so the
    // user isn't left wondering why nothing happened — without this,
    // hidden-tab fields (e.g. Media) can fail silently.
    const firstField = Object.keys(errors)[0] ?? "form";
    const firstMsg = (() => {
      const e = errors[firstField] as { message?: string } | undefined;
      return e?.message ?? "Please review the highlighted fields.";
    })();
    toast.error("Couldn't save — check the form", firstMsg);
  };

  const title =
    mode === "create" ? "New product" : (initialProduct?.title ?? "Product");


  // --- Create -------------------------------------------------------------
  // A short form that makes the product exist, then redirects into edit.
  //
  // It deliberately does NOT reuse the edit page's furniture. No rail: the
  // rail holds metadata worth glancing at while scrolling a long page, and
  // four fields have no scroll to glance across — building one anyway would
  // be cargo-culting a shape whose justification is absent. No docked action
  // bar either, for the same reason: that bar exists to answer "did I save"
  // across a long scroll, and there is no long scroll here.
  //
  // max-w-2xl with NO mx-auto. AdminShell already owns the page width; the
  // empty space to the right is the asymmetric margin this system asks for,
  // not a mistake to correct by centring. Centring it would land straight on
  // the generic signup-card look the design direction rules out.
  //
  // Media is absent rather than disabled — before the product exists there is
  // nothing to attach an upload to, and a disabled dropzone is a promise the
  // page cannot keep.
  if (mode === "create") {
    return (
      <FormProvider {...methods}>
        <form
          onSubmit={methods.handleSubmit(onSubmit, onInvalid)}
          className="flex flex-col gap-8"
          aria-labelledby="product-form-heading"
        >
          <header className="flex flex-col gap-3">
            <h1
              id="product-form-heading"
              className="font-serif text-2xl font-medium leading-tight tracking-tight text-foreground sm:text-3xl"
            >
              New product
            </h1>
          </header>

          {rootError && (
            <div
              role="alert"
              className="border-y border-[color:var(--danger)]/30 bg-[color:var(--danger)]/[0.06] px-4 py-3 text-sm text-[color:var(--danger)]"
            >
              {rootError}
            </div>
          )}

          <div className="flex max-w-2xl flex-col gap-10">
            <ProductSection
              id="details"
              title="Details"
              description="You can add the description, media and options once it exists."
            >
              <div className="flex flex-col gap-6">
                <DetailsSection mode="create" compact />
                <Field label="Categories" error={methods.formState.errors.categoryIds?.message}>
                  <ProductCategoriesPicker
                    allCategories={categories}
                    selectedIds={methods.watch("categoryIds")}
                    onChange={(ids: string[]) =>
                      methods.setValue("categoryIds", ids, { shouldDirty: true })
                    }
                    storeId={storeId}
                  />
                </Field>
              </div>
            </ProductSection>

            <ProductSection
              id="pricing"
              title="Pricing and inventory"
              description="A starting price and count. Both are editable straight after."
            >
              <PricingSection
                mode="create"
                currencyCode={currencyCode}
                hasMultipleVariants={false}
                storeId={storeId}
              />
            </ProductSection>

            {/* Status is folded into the two actions rather than asked as a
                fifth field. The merchant decides it by choosing how to save,
                and asking twice — once as a select, once implicitly by
                submitting — is the redundancy this redesign has been removing.
                Both are inline-sized and flush left, so the page stays
                asymmetric to its last element instead of resolving into a
                centred footer. */}
            <div className="flex flex-wrap items-center gap-4 border-t border-border-subtle pt-8">
              <button
                type="submit"
                disabled={isPending}
                onClick={() => methods.setValue("status", "draft")}
                className="inline-flex items-center justify-center rounded-md bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
              >
                {isPending ? "Creating…" : "Create as draft"}
              </button>
              <button
                type="submit"
                disabled={isPending}
                onClick={() => methods.setValue("status", "active")}
                className="inline-flex items-center justify-center rounded-md border border-[color:var(--ink-200)] px-5 py-2 text-sm font-medium text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
              >
                Create and publish
              </button>
              <Link
                href="/products"
                onClick={handleDiscard}
                // Owns its own confirmation, as on the edit page.
                data-unsaved-guard="off"
                className="text-sm text-foreground-secondary underline-offset-4 transition-colors hover:text-[color:var(--moss-700)] hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              >
                Cancel
              </Link>
            </div>
          </div>
        </form>

        <AlertDialog
          isOpen={discardOpen}
          onClose={() => setDiscardOpen(false)}
          title="Discard this product?"
          message="Nothing has been created yet."
          type="confirm"
          confirmLabel="Discard"
          cancelLabel="Keep editing"
          onConfirm={confirmDiscard}
          onCancel={() => setDiscardOpen(false)}
        />
      </FormProvider>
    );
  }

  return (
    <FormProvider {...methods}>
      <form
        onSubmit={methods.handleSubmit(onSubmit, onInvalid)}
        className="flex flex-col gap-8"
        aria-labelledby="product-form-heading"
      >
        <header className="flex flex-col gap-3">
          <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-2">
            <h1
              id="product-form-heading"
              className="font-serif text-2xl font-medium leading-tight tracking-tight text-foreground sm:text-3xl"
            >
              {title}
            </h1>
            {mode === "edit" && initialProduct && (
              <StatusDot status={initialProduct.status} />
            )}
          </div>
          {mode === "edit" && initialProduct && (
            <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-foreground-tertiary">
              <span className="font-mono">/{initialProduct.handle}</span>
              {storeSlug && initialProduct.status === "active" && (
                <>
                  <span aria-hidden="true">·</span>
                  <a
                    href={`${
                      typeof window !== "undefined" && window.location.hostname === "localhost"
                        ? `/products/${initialProduct.handle}?slug=${storeSlug}`
                        : `https://${storeSlug}.mark8ly.com/products/${initialProduct.handle}`
                    }`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-[color:var(--moss-700)] transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                  >
                    View on storefront ↗
                  </a>
                </>
              )}
            </p>
          )}
        </header>

        {rootError && (
          <div
            role="alert"
            className="border-y border-[color:var(--danger)]/30 bg-[color:var(--danger)]/[0.06] px-4 py-3 text-sm text-[color:var(--danger)]"
          >
            {rootError}
          </div>
        )}

        {/* The docked action bar.
            Save used to live in the rail. The merchant's report was that it
            "dangled in the middle of the page", and that was an anchoring
            problem, not a boundary one — the rail is `self-start`, so it is a
            short box floating beside a long column, and no amount of border
            fixes that. Here Save has a fixed home: it stays put under the
            topbar at every scroll position.

            Flat --background, one hairline underneath, no shadow and no
            backdrop-blur. The bar is part of the page rather than floating
            above it, which is what keeps it from reading as dashboard chrome.

            top is --admin-topbar-h, published by AdminShell on the ancestor of
            both its own sticky topbar and <main>. The topbar is `sticky top-0
            z-30`, so a bar at top-0 would slide underneath it; docking to the
            shared variable means the two cannot drift apart. z-20 sits below
            the topbar deliberately — they abut rather than overlap.

            The negative margins let the hairline run the full content width
            while the controls stay on the page's own left edge. */}
        <div className="sticky top-[var(--admin-topbar-h)] z-20 -mx-4 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b border-border-subtle bg-background px-4 py-3 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
          <div className="flex items-center gap-4">
            <button
              type="submit"
              disabled={isPending}
              className="inline-flex items-center justify-center gap-2 rounded-md bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
            >
              {isPending ? "Saving…" : "Save changes"}
            </button>
            <Link
              href="/products"
              onClick={handleDiscard}
              // This link already opens its own styled confirmation, so the
              // global navigation guard steps aside rather than stacking a
              // second dialog on the same click.
              data-unsaved-guard="off"
              className="text-sm text-foreground-secondary underline-offset-4 transition-colors hover:text-[color:var(--moss-700)] hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              Discard
            </Link>
            <span className="hidden text-xs text-[color:var(--ink-900)]/40 sm:inline">
              Changes apply when you save.
            </span>
          </div>
          {mode === "edit" && canDelete && initialProduct && (
            <button
              type="button"
              onClick={handleDelete}
              disabled={isPending}
              className="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-sm text-foreground-secondary transition-colors hover:text-[color:var(--danger)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
              aria-label="Delete product"
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" /> Delete
            </button>
          )}
        </div>

        <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_18rem] lg:gap-14">
          <div className="flex flex-col gap-10">
            <ProductSection
              id="details"
              title="Details"
              description="What the product is called, and how it reads on the storefront."
            >
              <DetailsSection mode={mode} />
            </ProductSection>

            <ProductSection
              id="pricing"
              title={hasMultipleVariants ? "Variants" : "Pricing and inventory"}
              description={
                !hasMultipleVariants
                  ? "What it costs, and how many you have."
                  : hasOptions
                    ? "Each combination has its own price, stock and SKU."
                    : "Each variant has its own price, stock and SKU."
              }
            >
              <PricingSection
                mode={mode}
                currencyCode={currencyCode}
                hasMultipleVariants={hasMultipleVariants}
                storeId={storeId}
                productId={initialProduct?.id}
                variantId={firstVariant?.id}
                warehouses={warehouses}
                stockByLocation={firstVariant?.inventory_by_location ?? {}}
                stockByVariant={stockByVariant}
              />
            </ProductSection>

            <ProductSection
              id="options"
              title="Options"
              description={
                !hasMultipleVariants
                  ? "Add size, colour or similar to sell this product in several variations."
                  : hasOptions
                    ? "Size, colour and the like. Changing these regenerates the variants above."
                    : // These variants were never built from options, so no
                      // option value can match them: generateVariants returns
                      // every existing id in removedIds and Save deletes them.
                      // Saying so is the difference between an informed choice
                      // and losing a catalogue.
                      "These variants have no options. Adding one rebuilds the list above from scratch and replaces them."
              }
            >
              <OptionsTab />
            </ProductSection>

            <ProductSection
              id="shipping"
              title="Shipping"
              description="Used to quote carrier rates. Wrong numbers cost you the difference on every order."
            >
              <ShippingSection />
            </ProductSection>

            {/* Media needs a product to attach to, so it is ABSENT on
                create rather than present-and-disabled. A section that
                cannot work yet is worse than one that is not there. */}
            {mode === "edit" && initialProduct && (
              <ProductSection
                id="media"
                title="Media"
                description="The photos shoppers see. The first is used in listings."
              >
                <MediaTab
                  storeId={storeId}
                  productId={initialProduct.id}
                  session={session}
                />
              </ProductSection>
            )}

            <TaxSection storeCountryCode={storeCountryCode} />
          </div>

          <ProductRail categories={categories} storeId={storeId} />
        </div>
      </form>

      {initialProduct && (
        <AlertDialog
          isOpen={deleteOpen}
          onClose={() => setDeleteOpen(false)}
          title={`Delete "${initialProduct.title}"?`}
          message="This can't be undone."
          type="confirm"
          confirmLabel="Delete"
          cancelLabel="Cancel"
          onConfirm={confirmDelete}
          onCancel={() => setDeleteOpen(false)}
        />
      )}

      <AlertDialog
        isOpen={discardOpen}
        onClose={() => setDiscardOpen(false)}
        title="Discard unsaved changes?"
        message="Your edits will be lost."
        type="confirm"
        confirmLabel="Discard"
        cancelLabel="Keep editing"
        onConfirm={confirmDiscard}
        onCancel={() => setDiscardOpen(false)}
      />
    </FormProvider>
  );
}

// Internal helper: subscribe to a field via RHF's useWatch without a
// separate component. Keeps badge counts live without triggering loops.
function useWatchSafe(
  methods: ReturnType<typeof useForm<ProductFormValues>>,
  name: "media" | "variants",
) {
  return useWatch({ control: methods.control, name });
}

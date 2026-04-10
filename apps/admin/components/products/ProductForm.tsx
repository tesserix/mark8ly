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
import { ArrowLeft, Trash2 } from "lucide-react";

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
} from "@/app/products/actions";
import {
  generateVariants,
  type OptionDraft as GenOptionDraft,
} from "@/lib/products/generateVariants";
import type { VariantDraft as MatrixVariantDraft } from "@/components/products/variants/VariantMatrixTable";

import {
  ProductFormTabs,
  type ProductFormTabId,
} from "./form/ProductFormTabs";
import { GeneralTab } from "./form/GeneralTab";
import { OptionsTab } from "./form/OptionsTab";
import { VariantsTab } from "./form/VariantsTab";
import { MediaTab } from "./form/MediaTab";

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
  canDelete: boolean;
  canArchive: boolean;
  session: SessionHeaders;
}

export function ProductForm({
  mode,
  storeId,
  initialProduct,
  categories,
  currencyCode,
  canDelete,
  session,
}: ProductFormProps) {
  const router = useRouter();
  const [isPending, startTransition] = useTransition();
  const [rootError, setRootError] = useState<string | null>(null);
  const [currentTab, setCurrentTab] = useState<ProductFormTabId>("general");
  const hasMultipleVariants = (initialProduct?.variants.length ?? 0) > 1;
  const firstVariant = initialProduct?.variants[0];

  const defaults: ProductFormValues = {
    title: initialProduct?.title ?? "",
    handle: initialProduct?.handle ?? "",
    description: initialProduct?.description ?? "",
    status: initialProduct?.status ?? "draft",
    price: firstVariant?.price ?? "",
    inventoryQuantity: firstVariant
      ? String(firstVariant.inventory_quantity)
      : "0",
    sku: firstVariant?.sku ?? "",
    categoryIds: initialProduct?.categories.map((c) => c.id) ?? [],
    options: [],
    variants: [],
    media: [],
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

  useEffect(() => {
    if (!options) return;

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
        // Derive optionValues by parsing the key "name::value|name::value"
        const optionValues = v.key
          .split("|")
          .map((pair) => {
            const [optionName, value] = pair.split("::");
            return { optionName: optionName ?? "", value: value ?? "" };
          })
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
          router.refresh();
        }
      }
    });
  };

  const handleDelete = () => {
    if (!initialProduct) return;
    if (
      !window.confirm(
        `Delete "${initialProduct.title}"? This cannot be undone.`,
      )
    )
      return;
    startTransition(async () => {
      const result = await deleteProductAction(storeId, initialProduct.id);
      if (!result.ok && result.error) {
        setRootError(result.error.message);
      }
    });
  };

  const title =
    mode === "create" ? "New product" : (initialProduct?.title ?? "Product");

  // Media tab is disabled in create mode — MediaTab requires a productId
  // and a session for upload/recrop calls.
  const tabDisabled: Partial<Record<ProductFormTabId, string>> =
    mode === "create"
      ? { media: "Save the product first to upload images" }
      : {};

  const mediaCount =
    (useWatchSafe(methods, "media") as unknown[] | undefined)?.length ?? 0;
  const variantCount =
    (useWatchSafe(methods, "variants") as unknown[] | undefined)?.length ?? 0;

  return (
    <FormProvider {...methods}>
      <form
        onSubmit={methods.handleSubmit(onSubmit)}
        className="flex flex-col gap-8"
        aria-labelledby="product-form-heading"
      >
        {/* Header strip */}
        <div className="flex flex-col gap-2">
          <Link
            href="/products"
            className="inline-flex w-fit items-center gap-1 text-sm text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            <ArrowLeft className="h-4 w-4" aria-hidden="true" /> Products
          </Link>
          <div className="flex items-start justify-between gap-4">
            <h1
              id="product-form-heading"
              className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl leading-tight text-[color:var(--ink-900)]"
            >
              {title}
            </h1>
            {mode === "edit" && initialProduct && (
              <span className="text-xs text-[color:var(--ink-900)] opacity-50">
                /{initialProduct.handle}
              </span>
            )}
          </div>
        </div>

        {rootError && (
          <div
            role="alert"
            className="rounded-md border border-[color:var(--signal,#C23B22)] border-opacity-40 bg-[color:var(--signal,#C23B22)] bg-opacity-5 px-4 py-3 text-sm text-[color:var(--signal,#C23B22)]"
          >
            {rootError}
          </div>
        )}

        <ProductFormTabs
          active={currentTab}
          onChange={setCurrentTab}
          badges={{ media: mediaCount, variants: variantCount }}
          disabled={tabDisabled}
        />

        <div>
          {currentTab === "general" && (
            <GeneralTab
              mode={mode}
              categories={categories}
              currencyCode={currencyCode}
              hasMultipleVariants={hasMultipleVariants}
            />
          )}
          {currentTab === "media" && mode === "edit" && initialProduct && (
            <MediaTab
              storeId={storeId}
              productId={initialProduct.id}
              session={session}
            />
          )}
          {currentTab === "options" && <OptionsTab />}
          {currentTab === "variants" && (
            <VariantsTab currencyCode={currencyCode} />
          )}
        </div>

        {/* Actions */}
        <div className="flex items-center justify-between border-t border-[color:var(--ink-900)] border-opacity-10 pt-6">
          <Link
            href="/products"
            className="text-sm text-[color:var(--ink-900)] opacity-70 underline-offset-4 hover:underline focus-visible:underline focus-visible:outline-none"
          >
            Discard
          </Link>
          <div className="flex items-center gap-3">
            {mode === "edit" && canDelete && initialProduct && (
              <button
                type="button"
                onClick={handleDelete}
                disabled={isPending}
                className="inline-flex items-center gap-1 rounded-md border border-[color:var(--ink-900)] border-opacity-20 px-3 py-2 text-sm text-[color:var(--signal,#C23B22)] transition-colors hover:bg-[color:var(--signal,#C23B22)] hover:bg-opacity-5 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
                aria-label="Delete product"
              >
                <Trash2 className="h-4 w-4" aria-hidden="true" /> Delete
              </button>
            )}
            <button
              type="submit"
              disabled={isPending}
              className="inline-flex items-center gap-2 rounded-md bg-[color:var(--moss-700)] px-5 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
            >
              {isPending
                ? "Saving…"
                : mode === "create"
                  ? "Create product"
                  : "Save changes"}
            </button>
          </div>
        </div>
      </form>
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

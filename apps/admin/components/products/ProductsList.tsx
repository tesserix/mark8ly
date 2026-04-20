"use client";

// apps/admin/components/products/ProductsList.tsx
//
// The products data-table. M7d: wired with useProductSelection hook,
// BulkActionsBar, CopyToStoreDialog (single + bulk), BulkDeleteConfirmDialog,
// and BulkCategoryAssignPopover. Overflow menu per row with "Copy to store".

import { useState, useCallback, useMemo, memo } from "react";
import Link from "next/link";
import Image from "next/image";
import type { ReactNode } from "react";
import { MoreHorizontal, Copy } from "lucide-react";

import { StatusDot } from "@repo/ui/status-dot";
import { PriceDisplay } from "@repo/ui/price-display";

import type {
  AdminProduct,
  AdminMediaResponse,
  AdminStore,
  AdminCategory,
} from "@/lib/api/marketplace-api";
import { useProductSelection } from "@/lib/products/useProductSelection";
import { BulkActionsBar, type StoreRole } from "./BulkActionsBar";
import { CopyToStoreDialog, type CopyToStoreRequest } from "./CopyToStoreDialog";
import { BulkDeleteConfirmDialog } from "./BulkDeleteConfirmDialog";
import { BulkCategoryAssignPopover } from "./BulkCategoryAssignPopover";
import {
  copyProductAction,
  bulkArchiveAction,
  bulkUnarchiveAction,
  bulkPublishAction,
  bulkUnpublishAction,
  bulkDeleteAction,
  bulkAssignCategoryAction,
} from "@/app/(admin)/products/actions";

export interface ProductsListProps {
  products: AdminProduct[];
  storeId: string;
  role: StoreRole;
  stores: AdminStore[];
  categories: AdminCategory[];
}

export function ProductsList({
  products,
  storeId,
  role,
  stores,
  categories,
}: ProductsListProps) {
  const selection = useProductSelection(storeId);

  // Copy dialog state
  const [copyProductIds, setCopyProductIds] = useState<string[]>([]);
  const [isCopyOpen, setIsCopyOpen] = useState(false);

  // Delete confirm state
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);

  // Category assign state
  const [isCategoryOpen, setIsCategoryOpen] = useState(false);

  // Overflow menu state
  const [openMenuId, setOpenMenuId] = useState<string | null>(null);

  const allProductIds = useMemo(() => products.map((p) => p.id), [products]);
  const allSelected = useMemo(
    () => products.length > 0 && products.every((p) => selection.isSelected(p.id)),
    [products, selection],
  );

  const handleSelectAll = useCallback(() => {
    if (allSelected) {
      selection.clearAll();
    } else {
      selection.selectAll(allProductIds);
    }
  }, [allSelected, allProductIds, selection]);

  const handleCopySingle = useCallback(
    (productId: string) => {
      setCopyProductIds([productId]);
      setIsCopyOpen(true);
      setOpenMenuId(null);
    },
    [],
  );

  const handleCopyBulk = useCallback(() => {
    setCopyProductIds([...selection.selectedIds]);
    setIsCopyOpen(true);
  }, [selection.selectedIds]);

  const handleCopySubmit = useCallback(
    async (req: CopyToStoreRequest) => {
      for (const id of req.productIds) {
        await copyProductAction(storeId, id, {
          targetStoreId: req.targetStoreId,
          copyMedia: req.copyMedia,
        });
      }
      setIsCopyOpen(false);
      selection.clearAll();
    },
    [storeId, selection],
  );

  const handleBulkAction = useCallback(
    async (action: string) => {
      const ids = [...selection.selectedIds];
      const actionMap: Record<string, (s: string, ids: string[]) => Promise<unknown>> = {
        archive: bulkArchiveAction,
        unarchive: bulkUnarchiveAction,
        publish: bulkPublishAction,
        unpublish: bulkUnpublishAction,
      };
      const fn = actionMap[action];
      if (fn) {
        await fn(storeId, ids);
        selection.clearAll();
      }
    },
    [storeId, selection],
  );

  const handleDeleteConfirm = useCallback(async () => {
    await bulkDeleteAction(storeId, [...selection.selectedIds]);
    setIsDeleteOpen(false);
    selection.clearAll();
  }, [storeId, selection]);

  const handleCategoryApply = useCallback(
    async (categoryIds: string[]) => {
      await bulkAssignCategoryAction(
        storeId,
        [...selection.selectedIds],
        categoryIds,
      );
      setIsCategoryOpen(false);
      selection.clearAll();
    },
    [storeId, selection],
  );

  return (
    <div className="w-full">
      <div className="table-responsive">
      <table
        className="w-full border-collapse text-sm"
        aria-label="Products"
      >
        <thead>
          <tr className="border-b border-[color:var(--ink-900)]/10 text-left text-xs font-medium uppercase tracking-wide text-foreground-tertiary">
            <th scope="col" className="w-10 py-3 pl-4 pr-2">
              <input
                type="checkbox"
                aria-label="Select all products"
                checked={allSelected}
                onChange={handleSelectAll}
                className="h-4 w-4 rounded border-[color:var(--ink-900)]/30 text-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]"
              />
            </th>
            <th scope="col" className="w-14 py-3 px-2" aria-hidden="true" />
            <th scope="col" className="py-3 px-3">
              Product
            </th>
            <th scope="col" className="w-32 py-3 px-3">
              Status
            </th>
            <th scope="col" className="w-40 py-3 px-3">
              Stock
            </th>
            <th scope="col" className="w-32 py-3 px-3">
              Price
            </th>
            <th scope="col" className="w-10 py-3 pl-2 pr-4" aria-hidden="true" />
          </tr>
        </thead>
        <tbody>
          {products.map((p) => (
            <ProductRow
              key={p.id}
              product={p}
              isSelected={selection.isSelected(p.id)}
              onToggle={() => selection.toggle(p.id)}
              isMenuOpen={openMenuId === p.id}
              onMenuToggle={() =>
                setOpenMenuId(openMenuId === p.id ? null : p.id)
              }
              onCopyToStore={() => handleCopySingle(p.id)}
            />
          ))}
        </tbody>
      </table>
      </div>

      <BulkActionsBar
        selectedIds={selection.selectedIds}
        role={role}
        storeId={storeId}
        categories={categories}
        onActionComplete={() => selection.clearAll()}
        onCopyClick={handleCopyBulk}
        onDeleteClick={() => setIsDeleteOpen(true)}
        onCategoryAssignClick={() => setIsCategoryOpen(true)}
        onBulkAction={handleBulkAction}
      />

      <CopyToStoreDialog
        isOpen={isCopyOpen}
        onClose={() => setIsCopyOpen(false)}
        stores={stores}
        currentStoreId={storeId}
        productIds={copyProductIds}
        onCopy={handleCopySubmit}
      />

      <BulkDeleteConfirmDialog
        isOpen={isDeleteOpen}
        count={selection.count}
        onConfirm={handleDeleteConfirm}
        onCancel={() => setIsDeleteOpen(false)}
      />

      <BulkCategoryAssignPopover
        isOpen={isCategoryOpen}
        categories={categories}
        count={selection.count}
        onApply={handleCategoryApply}
        onClose={() => setIsCategoryOpen(false)}
      />
    </div>
  );
}

const ProductRow = memo(function ProductRow({
  product,
  isSelected,
  onToggle,
  isMenuOpen,
  onMenuToggle,
  onCopyToStore,
}: {
  product: AdminProduct;
  isSelected: boolean;
  onToggle: () => void;
  isMenuOpen: boolean;
  onMenuToggle: () => void;
  onCopyToStore: () => void;
}) {
  const firstMedia = product.media[0];
  const variantCount = product.variants.length;

  const priceMin = product.variants.reduce<number | null>((acc, v) => {
    const n = Number.parseFloat(v.price);
    if (!Number.isFinite(n)) return acc;
    if (acc === null || n < acc) return n;
    return acc;
  }, null);
  const priceMax = product.variants.reduce<number | null>((acc, v) => {
    const n = Number.parseFloat(v.price);
    if (!Number.isFinite(n)) return acc;
    if (acc === null || n > acc) return n;
    return acc;
  }, null);
  const currency = product.variants[0]?.currency_code ?? "USD";
  const isVariantRange =
    priceMin !== null && priceMax !== null && priceMin !== priceMax;

  const stock = product.variants.reduce(
    (sum, v) => sum + v.inventory_quantity,
    0,
  );
  const isDraft = product.status === "draft";
  const isLowStock = stock > 0 && stock <= 5;
  const isOutOfStock = stock === 0;

  return (
    <tr className="h-14 border-b border-[color:var(--ink-900)]/5 transition-colors hover:bg-[color:var(--ink-900)]/[0.03]">
      <td className="py-3 pl-4 pr-2">
        <input
          type="checkbox"
          aria-label={`Select ${product.title}`}
          checked={isSelected}
          onChange={onToggle}
          className="h-4 w-4 rounded border-[color:var(--ink-900)]/30 text-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]"
        />
      </td>
      <td className="py-3 px-2">
        <MediaThumb media={firstMedia} productTitle={product.title} />
      </td>
      <td className="py-3 px-3">
        <Link
          href={`/products/${product.id}`}
          className="group inline-flex flex-col rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <span className="font-serif text-base text-foreground group-hover:underline">
            {product.title}
          </span>
          <span className="text-xs text-foreground-tertiary">
            /{product.handle}
          </span>
        </Link>
      </td>
      <td className="py-3 px-3">
        <StatusDot status={product.status} />
      </td>
      <td className="py-3 px-3">
        <StockCell
          stock={stock}
          variantCount={variantCount}
          isDraft={isDraft}
          isLowStock={isLowStock}
          isOutOfStock={isOutOfStock}
        />
      </td>
      <td className="py-3 px-3">
        <PriceCell
          priceMin={priceMin}
          isVariantRange={isVariantRange}
          currency={currency}
        />
      </td>
      <td className="relative py-3 pl-2 pr-4 text-right">
        <button
          type="button"
          aria-label={`More actions for ${product.title}`}
          aria-haspopup="menu"
          aria-expanded={isMenuOpen}
          onClick={onMenuToggle}
          className="inline-flex h-8 w-8 items-center justify-center rounded-md text-foreground-secondary hover:bg-[color:var(--ink-900)]/5 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
        </button>
        {isMenuOpen && (
          <div
            role="menu"
            className="absolute right-0 top-full z-20 mt-1 min-w-[160px] rounded-md border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--background-elevated,white)] py-1 shadow-[var(--shadow-2,0_4px_12px_rgba(0,0,0,0.08))]"
          >
            <button
              type="button"
              role="menuitem"
              onClick={onCopyToStore}
              className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-[color:var(--ink-900)] hover:bg-[color:var(--ink-900)]/5"
            >
              <Copy className="h-3.5 w-3.5" aria-hidden="true" />
              Copy to store
            </button>
          </div>
        )}
      </td>
    </tr>
  );
});

function MediaThumb({
  media,
  productTitle,
}: {
  media: AdminMediaResponse | undefined;
  productTitle: string;
}) {
  if (!media) {
    return (
      <div
        className="h-10 w-10 rounded border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--paper-200)]"
        aria-hidden="true"
      />
    );
  }
  return (
    <div className="relative h-10 w-10 overflow-hidden rounded border border-[color:var(--ink-900)] border-opacity-10">
      <Image
        src={media.url}
        alt={media.alt ?? productTitle}
        fill
        sizes="40px"
        unoptimized
      />
    </div>
  );
}

function StockCell({
  stock,
  variantCount,
  isDraft,
  isLowStock,
  isOutOfStock,
}: {
  stock: number;
  variantCount: number;
  isDraft: boolean;
  isLowStock: boolean;
  isOutOfStock: boolean;
}): ReactNode {
  if (isDraft) {
    return (
      <span className="text-[color:var(--ink-900)] opacity-40">Draft</span>
    );
  }
  if (isOutOfStock) {
    return (
      <span className="text-[color:var(--signal)]">Out of stock</span>
    );
  }
  if (isLowStock) {
    return (
      <span className="text-[color:var(--signal)]">
        {stock} in stock
      </span>
    );
  }
  return (
    <span className="text-[color:var(--ink-900)]">
      {stock} {variantCount > 1 ? `across ${variantCount} variants` : "in stock"}
    </span>
  );
}

function PriceCell({
  priceMin,
  isVariantRange,
  currency,
}: {
  priceMin: number | null;
  isVariantRange: boolean;
  currency: string;
}): ReactNode {
  if (priceMin === null) {
    return <span className="text-[color:var(--ink-900)] opacity-40">—</span>;
  }
  if (isVariantRange) {
    return (
      <span className="text-[color:var(--ink-900)]">
        from <PriceDisplay amount={String(priceMin)} currencyCode={currency} />
      </span>
    );
  }
  return <PriceDisplay amount={String(priceMin)} currencyCode={currency} />;
}

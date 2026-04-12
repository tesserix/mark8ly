// apps/storefront/app/products/[handle]/page.tsx
//
// Storefront product detail page. Server component fetches the product,
// then hands off to client components for interactive variant selection,
// media gallery, and add-to-cart.

import Link from "next/link";
import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { notFound } from "next/navigation";

import { fetchStoreBySlug } from "@/lib/api/platform-api";
import { getProductByHandle } from "@/lib/api/marketplace-api";
import { slugFromHost } from "@/lib/slug";
import { hasSessionCookie } from "@/lib/auth";
import { StorefrontNav } from "@/components/StorefrontNav";
import { MediaGallery } from "@/components/MediaGallery";
import { ProductDetails } from "@/components/ProductDetails";
import { ProductReviews } from "@/components/reviews/ProductReviews";

export const dynamic = "force-dynamic";

interface PageProps {
  params: Promise<{ handle: string }>;
  searchParams: Promise<{ slug?: string }>;
}

async function resolveSlug(query: { slug?: string }): Promise<string> {
  const h = await headers();
  const host = h.get("host");
  return (
    query.slug ||
    slugFromHost(host) ||
    process.env.DEFAULT_STORE_SLUG ||
    ""
  );
}

export async function generateMetadata({
  params,
  searchParams,
}: PageProps): Promise<Metadata> {
  const { handle } = await params;
  const sp = await searchParams;
  const slug = await resolveSlug(sp);
  if (!slug) return { title: "Product" };
  const product = await getProductByHandle(slug, handle);
  if (!product) return { title: "Product not found" };
  return {
    title: product.seo_title ?? product.title,
    description: product.seo_description ?? product.description ?? undefined,
  };
}

export default async function StorefrontProductPage({
  params,
  searchParams,
}: PageProps) {
  const { handle } = await params;
  const sp = await searchParams;
  const slug = await resolveSlug(sp);

  const store = slug ? await fetchStoreBySlug(slug) : null;
  if (!store) notFound();

  const product = await getProductByHandle(slug, handle);
  if (!product) notFound();

  return (
    <main id="main" className="min-h-screen bg-[color:var(--paper-200)]">
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <StorefrontNav storeName={store.name} />
        <Link
          href="/products"
          className="mb-8 inline-flex items-center gap-1 text-xs font-semibold uppercase tracking-[0.18em] text-[color:var(--ink-900)] opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          ← Shop
        </Link>

        <div className="grid gap-12 lg:grid-cols-2">
          <MediaGallery media={product.media} productTitle={product.title} />
          <ProductDetails product={product} />
        </div>

        <ProductReviews
          productHandle={handle}
          isAuthenticated={hasSessionCookie((await cookies()).toString())}
        />
      </div>
    </main>
  );
}

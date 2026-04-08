import {
  Eyebrow,
  HeroTitle,
  LinkCta,
  PrimaryButton,
  ProductSlot,
  SecondaryButton,
  type LayoutProps,
} from "./shared";

/**
 * Split-hero — true 50/50 cinematic hero.
 *
 * Text rail on the left, full-bleed product image placeholder on the
 * right that runs edge-to-edge. Strong for product-led brands where
 * the imagery does the narrative lift.
 */
export function SplitHeroLayout({ store, theme }: LayoutProps) {
  return (
    <article className="space-y-16">
      <section
        className="-mx-6 grid min-h-[32rem] gap-0 sm:-mx-8 lg:grid-cols-2"
        style={{ background: `${theme.colors.surface}66` }}
      >
        <div className="flex flex-col justify-center space-y-6 px-6 py-12 sm:px-12 lg:py-20">
          <Eyebrow theme={theme}>Featured · Launch drop</Eyebrow>
          <HeroTitle store={store} theme={theme} size="lg" />
          <p className="max-w-md text-base leading-8 opacity-75">
            A cinematic opening frame — product on the right, narrative on the
            left. Equal weight, no competition.
          </p>
          <div className="flex flex-wrap items-center gap-4 pt-2">
            <PrimaryButton theme={theme} size="lg">
              Shop the drop
            </PrimaryButton>
            <SecondaryButton theme={theme}>Lookbook</SecondaryButton>
          </div>
        </div>
        <div className="relative min-h-[24rem]">
          <ProductSlot
            theme={theme}
            label="Featured piece"
            ratio="aspect-auto h-full"
            size="lg"
          />
        </div>
      </section>

      <section className="grid gap-6 sm:grid-cols-3">
        {["Bestseller", "Restocked", "Staff pick"].map((label) => (
          <div key={label} className="space-y-3">
            <ProductSlot theme={theme} label={label} ratio="aspect-square" />
            <LinkCta theme={theme}>Shop now</LinkCta>
          </div>
        ))}
      </section>
    </article>
  );
}

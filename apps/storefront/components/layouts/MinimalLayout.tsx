import {
  Eyebrow,
  HeroTitle,
  LinkCta,
  PrimaryButton,
  ProductSlot,
  type LayoutProps,
} from "./shared";

/**
 * Minimal — huge negative space, one frame, one CTA.
 *
 * For merchants with extreme confidence in their imagery. A centered
 * mark, a massive product frame, a single action. Nothing else.
 */
export function MinimalLayout({ store, theme }: LayoutProps) {
  return (
    <article className="mx-auto max-w-4xl space-y-20 py-12 text-center">
      <div className="space-y-6">
        <Eyebrow theme={theme}>Now open</Eyebrow>
        <HeroTitle store={store} theme={theme} align="center" size="lg" />
      </div>

      <ProductSlot
        theme={theme}
        label="Featured"
        ratio="aspect-[16/10]"
        size="lg"
      />

      <div className="space-y-6">
        <p className="mx-auto max-w-xl text-lg leading-8 opacity-75">
          Fewer things, chosen carefully.
        </p>
        <div className="flex justify-center">
          <PrimaryButton theme={theme} size="lg">
            Enter the store
          </PrimaryButton>
        </div>
        <div className="flex justify-center">
          <LinkCta theme={theme}>About us</LinkCta>
        </div>
      </div>
    </article>
  );
}

import { CenteredCta, HeroTitle, type LayoutProps } from "./shared";

/**
 * Minimal layout — restrained, breathing room. Stub.
 */
export function MinimalLayout({ store, theme }: LayoutProps) {
  return (
    <section className="mx-auto max-w-3xl space-y-6 py-10 text-center">
      <HeroTitle store={store} theme={theme} align="center" />
      <p className="text-lg leading-8 opacity-75">
        A restrained storefront for merchants who want quiet confidence and
        plenty of breathing room.
      </p>
      <CenteredCta theme={theme} />
    </section>
  );
}

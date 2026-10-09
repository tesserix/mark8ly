// What the nav's brand slot shows.
//
// A page may pass the name it already fetched; otherwise the layout's
// resolved store supplies it. An empty string counts as "not supplied":
// checkout used to pass "" to avoid the literal "Store", which hid the
// brand entirely on the one page where a buyer most wants to know whose
// site they are paying. "Store" is the last resort, for an unresolvable
// host such as the 404 or sign-out pages.
export function resolveBrandName(
  fromPage: string | undefined,
  fromStore: string | null | undefined,
): string {
  const page = fromPage?.trim();
  if (page) return page;
  const store = fromStore?.trim();
  if (store) return store;
  return "Store";
}

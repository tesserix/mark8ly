// Add-to-cart label while the buyer has not finished picking a variant.
export function chooseOptionsLabel(optionNames: string[]): string {
  const names = optionNames.map((n) => n.toLowerCase());
  const list =
    names.length > 1 ? `${names.slice(0, -1).join(", ")} and ${names.at(-1)}` : names[0];
  return `Choose your ${list}`;
}

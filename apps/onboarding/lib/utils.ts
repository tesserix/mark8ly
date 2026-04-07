import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

/** cn merges Tailwind classes with conflict resolution. The standard helper. */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

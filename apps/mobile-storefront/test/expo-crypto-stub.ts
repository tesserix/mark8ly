// Import-resolution stub for vitest. Node has the same API.
import { randomUUID as nodeRandomUUID } from "node:crypto";
export function randomUUID(): string {
  return nodeRandomUUID();
}

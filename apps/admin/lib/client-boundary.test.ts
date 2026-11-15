// A server component that imports a non-component value (a list, a helper)
// from a "use client" module gets a client reference, not the value, and
// crashes at render: "PAYMENT_METHODS.find is not a function" took down
// /sell in production. Vitest renders everything in one world and can't see
// that, so check the imports statically.
import { readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

const ROOT = path.resolve(__dirname, "..");
const DIRS = ["app", "components", "lib"];

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const p = path.join(dir, name);
    if (statSync(p).isDirectory()) return name === "node_modules" ? [] : files(p);
    return /\.(ts|tsx)$/.test(name) && !/\.test\.tsx?$/.test(name) ? [p] : [];
  });
}

const isClient = (src: string) => /^\s*(\/\/[^\n]*\n|\s)*["']use client["']/.test(src);
const isServerAction = (src: string) => /^\s*(\/\/[^\n]*\n|\s)*["']use server["']/.test(src);

function resolve(from: string, spec: string): string | null {
  const base = spec.startsWith("@/") ? path.join(ROOT, spec.slice(2)) : spec.startsWith(".") ? path.resolve(path.dirname(from), spec) : null;
  if (!base) return null;
  for (const c of [base + ".tsx", base + ".ts", path.join(base, "index.tsx"), path.join(base, "index.ts")]) {
    try {
      if (statSync(c).isFile()) return c;
    } catch {}
  }
  return null;
}

describe("server components import only components from client modules", () => {
  it("has no non-component value imports across the boundary", () => {
    const bad: string[] = [];
    for (const f of DIRS.flatMap((d) => files(path.join(ROOT, d)))) {
      const src = readFileSync(f, "utf8");
      if (isClient(src) || isServerAction(src)) continue;
      for (const m of src.matchAll(/import\s+(?!type\b)\{([^}]+)\}\s+from\s+["']([^"']+)["']/g)) {
        const target = resolve(f, m[2]!);
        if (!target || !isClient(readFileSync(target, "utf8"))) continue;
        for (const raw of m[1]!.split(",")) {
          const name = raw.trim().replace(/^type\s+/, "").split(/\s+as\s+/)[0]!;
          if (!name || raw.trim().startsWith("type ")) continue;
          // Components (PascalCase) cross fine; hooks only run in client
          // code, so a file that calls one is client-rendered already.
          if (!/^[A-Z][A-Za-z0-9]*[a-z]/.test(name) && !/^use[A-Z]/.test(name)) bad.push(`${path.relative(ROOT, f)} imports ${name} from ${m[2]}`);
        }
      }
    }
    expect(bad).toEqual([]);
  });
});

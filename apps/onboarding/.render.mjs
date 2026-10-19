import { chromium } from "playwright";
import { mkdirSync } from "node:fs";
const dir = "/private/tmp/claude-501/-Users-mayu-Projects-tesserix-mark8ly/fbf88fd5-b9fb-451d-8fdc-d03dbf23c5e0/scratchpad/film";
const only = process.argv[2] ? process.argv[2].split(",").map(Number) : null;
const FPS = 30;
const b = await chromium.launch({ channel: "chrome" });
const p = await b.newPage({ viewport: { width: 1920, height: 1080 } });
await p.goto("file://" + dir + "/film.html", { waitUntil: "networkidle" });
await p.evaluate(() => document.fonts.ready);
await p.evaluate(() => Promise.all([...document.images].map(i => i.decode())));
const out = only ? dir + "/preview" : dir + "/frames";
mkdirSync(out, { recursive: true });
const dur = await p.evaluate(() => window.DURATION);
const times = only ?? Array.from({ length: Math.round(dur * FPS) }, (_, i) => i / FPS);
for (const [i, t] of times.entries()) {
  await p.evaluate((t) => window.render(t), t);
  const name = only ? `t${t}.jpg` : `f${String(i).padStart(5, "0")}.jpg`;
  await p.screenshot({ path: `${out}/${name}`, type: "jpeg", quality: 92 });
}
if (!only) console.log(JSON.stringify(await p.evaluate(() => window.CUES)));
console.log("frames", times.length);
await b.close();

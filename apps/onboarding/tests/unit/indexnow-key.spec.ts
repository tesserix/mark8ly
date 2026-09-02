import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

import { INDEXNOW_KEY, INDEXNOW_KEY_FILE } from "../../lib/seo/indexnow";

/**
 * Guards the IndexNow ownership key (tesserix/mark8ly#603).
 *
 * The failure this prevents is silent: if the constant and the file on disk
 * ever disagree, every submission gets a 403 and nothing surfaces it — there
 * is no page to look wrong. So the two are pinned to each other here.
 */
const PUBLIC_DIR = path.join(__dirname, "../../public");

test("the key file exists at the protocol's root path and holds exactly the key", () => {
  const file = path.join(PUBLIC_DIR, `${INDEXNOW_KEY}.txt`);

  const contents = readFileSync(file, "utf8");

  // Exactly the key, no trailing newline: the spec asks for a file "listing
  // the key", and a bare key is the form every reference implementation
  // serves.
  expect(contents).toBe(INDEXNOW_KEY);
  expect(INDEXNOW_KEY_FILE).toBe(`/${INDEXNOW_KEY}.txt`);
});

test("the key matches the character set and length the protocol allows", () => {
  // "a minimum of 8 and a maximum of 128 hexadecimal characters ... only
  // lowercase (a-z), uppercase (A-Z), numbers (0-9), and dashes (-)."
  expect(INDEXNOW_KEY).toMatch(/^[a-zA-Z0-9-]{8,128}$/);
});

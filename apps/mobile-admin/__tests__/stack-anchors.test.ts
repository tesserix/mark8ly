// tsconfig scopes `types` to ["jest"], so Node's ambient globals are not
// available — inline `require` + a `__dirname` declaration, matching
// rollout-invariants.test.ts and product-detail-sections.test.tsx.
declare const __dirname: string;

const fs = require("fs");
const path = require("path");
const APP_ROOT: string = path.join(__dirname, "..");
const APP_DIR: string = path.join(APP_ROOT, "app");

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true }) as Array<{
    name: string;
    isDirectory(): boolean;
  }>) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full));
    else if (entry.name === "_layout.tsx") out.push(full);
  }
  return out;
}

/**
 * DERIVED corpus, not a hand-written list — a layout added tomorrow is covered
 * with no edit here. That is this increment's own recorded lesson: "a coverage
 * test with a hand-copied list is not a coverage test."
 *
 * The rule: a layout that renders a <Stack> AND owns a sibling index.tsx must
 * declare an anchor. Without one, entering that stack at a nested route (the
 * Dashboard's NEEDS YOU queue, notifications.tsx, a push-payload deep link, or
 * any external mark8ly-admin:// link) leaves the stack holding only the nested
 * route — so Back exits the tab and the list screen becomes unreachable.
 *
 * Layouts WITHOUT a sibling index.tsx are NOT exempt — that exemption was the
 * hole this suite used to have. `more/settings` was a bare <Stack> with no
 * index and no anchor, so popping its only screen left the navigator holding
 * NOTHING. An empty stack cannot render: Back fell out to the Dashboard and
 * the More tab was left showing nothing when navigated to. Reported from a
 * device as "notification screen takes back to dashboard not prev screen and
 * more screen becomes inaccessible". The layout was deleted; those screens now
 * sit in the More stack, which is anchored.
 *
 * So the rule is: a <Stack> layout must have a floor route beneath whatever it
 * pushes. Owning an index and anchoring to it is how; owning no index at all
 * is the failure, not the exemption.
 *
 * The ROOT app/_layout.tsx is the one real exception: it owns no index, but it
 * is the bottom of the tree and nothing ever pops it empty — its children are
 * (tabs)/login/notifications.
 */
const ROOT_LAYOUT = path.join(APP_DIR, "_layout.tsx");

const STACK_LAYOUTS = walk(APP_DIR).filter((file) => {
  const src = fs.readFileSync(file, "utf8") as string;
  return /<Stack[\s/>]/.test(src) && file !== ROOT_LAYOUT;
});

const STACK_LAYOUTS_WITH_INDEX = STACK_LAYOUTS.filter((file) =>
  fs.existsSync(path.join(path.dirname(file), "index.tsx")),
);

describe("every tab stack that owns an index route declares it as the anchor", () => {
  it("found the stack layouts to check (guards against the corpus silently emptying)", () => {
    // A derived corpus that resolves to [] would make every assertion below
    // vacuous — the exact "test that cannot fail" shape this repo has already
    // been bitten by. Pin a floor instead.
    expect(STACK_LAYOUTS_WITH_INDEX.length).toBeGreaterThanOrEqual(5);
  });

  it("no Stack layout is left without a floor route to pop back to", () => {
    // The device bug in one assertion: a <Stack> whose directory owns no
    // index has nothing beneath its pushed screens, so popping the last one
    // empties the navigator.
    const floorless = STACK_LAYOUTS.filter(
      (f) => !fs.existsSync(path.join(path.dirname(f), "index.tsx")),
    ).map((f) => path.relative(APP_ROOT, f));

    expect(floorless).toEqual([]);
  });

  it.each(STACK_LAYOUTS_WITH_INDEX.map((f) => [path.relative(APP_ROOT, f), f]))(
    "%s anchors to index",
    (_label: string, file: string) => {
      const src = fs.readFileSync(file, "utf8") as string;
      expect(src).toMatch(/export\s+const\s+unstable_settings\s*=/);
      expect(src).toMatch(/initialRouteName:\s*['"]index['"]/);
    },
  );
});

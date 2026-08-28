# #372 — The encodeURIComponent divergence is six characters, and no vector covers five of them

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `signature.go`'s package doc name the full set of characters where `url.QueryEscape` diverges from `encodeURIComponent`, and add a golden vector that actually exercises the five the existing four vectors miss.

**Architecture:** No production code changes — `CanonicalQuery` is correct by definition, since `url.QueryEscape` *is* the spec. This is a documentation and test-vector change: extend `cmd/genvectors`, regenerate `testdata/vectors.json` from it, and correct the package doc.

**Tech Stack:** Go 1.26, HMAC-SHA256, `net/url`.

**Spec:** GitHub issue tesserix/mark8ly#372, plus the verified findings below.

## Global Constraints

- Run all Go commands from `services/marketplace-api`, never path-scoped, always `-count=1`.
- Required command set before commit: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.
- Commits: conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.
- Stage with explicit paths (`git add <path>`). Never `git add -A`.
- **`testdata/vectors.json` is GENERATED. Never hand-edit it, and never hand-compute a signature.** Regenerate it with the documented command and commit the output.
- **Do not change `CanonicalQuery` or any signing logic.** The existing four vectors' `canonical` and `signature` values must be byte-identical before and after — they are published on #275 and the console implements against them. Changing one silently breaks an external consumer.
- Work only inside this worktree: `.claude/worktrees/372-signature-doc-vectors`.

## Verified findings (established before planning — do not re-litigate)

1. **The issue's table is exactly right, and complete.** Verified empirically against Go 1.26.6 by escaping every ASCII punctuation character with both `url.QueryEscape` and an `encodeURIComponent` model (unreserved set `A-Za-z0-9 - _ . ! ~ * ' ( )`). Exactly six characters diverge:

   | char | `url.QueryEscape` | `encodeURIComponent` |
   |---|---|---|
   | space | `+` | `%20` |
   | `!` | `%21` | `!` |
   | `*` | `%2A` | `*` |
   | `'` | `%27` | `'` |
   | `(` | `%28` | `(` |
   | `)` | `%29` | `)` |

   `~ - _ .` agree; `+` is `%2B` on both sides. No character outside this set diverges.

2. **The four existing vectors genuinely miss five of the six.** Confirmed by reading `testdata/vectors.json`: `get-with-query` uses `since_hours`/`limit`; `post-with-body` has an empty `raw_query`; `repeated-query-and-encoded-path` uses `a`/`b`/`z`; `query-value-with-space` uses `Jane Smith`. None contains `!`, `*`, `'`, `(` or `)`. So a port applying only the documented `%20`→`+` fix passes all four and still 401s on `?actor=O'Brien`.

3. **The divergence applies to query KEYS as well as values — the issue says only "values".** `signature.go:117` is `url.QueryEscape(k)+"="+url.QueryEscape(v)`: both sides are escaped. The doc should say keys and values. This is a small extension to the issue, not a contradiction of it.

4. **Vectors are generated, not hand-written.** `internal/handlers/platformadmin/cmd/genvectors/main.go` builds them and its own doc comment gives the exact regeneration command. `signature_test.go:219-245` reads `testdata/vectors.json` and asserts both `canonical` and `signature` still match, so a hand-edited file would fail that test.

5. **No production code defect exists.** The issue says so itself and it is correct: `CanonicalQuery` is right because `url.QueryEscape` is the definition of the scheme. Do not "fix" the canonicaliser.

## File Structure

- `services/marketplace-api/internal/handlers/platformadmin/cmd/genvectors/main.go` — add the fifth vector's input.
- `services/marketplace-api/internal/handlers/platformadmin/testdata/vectors.json` — regenerated output (never hand-edited).
- `services/marketplace-api/internal/handlers/platformadmin/signature.go` — package doc: name all six characters, and say keys *and* values.

---

### Task 1: Add a vector covering the sub-delimiter characters

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/cmd/genvectors/main.go`
- Regenerate: `services/marketplace-api/internal/handlers/platformadmin/testdata/vectors.json`

**Interfaces:**
- Consumes: `platformadmin.SignatureInput{Method, Path, RawQuery, Body, Timestamp, Nonce, Operator, Capability}` (used by the four existing entries in `genvectors/main.go`).
- Produces: a fifth vector named `query-value-with-sub-delims`, consumed by `signature_test.go`'s table-driven golden test.

- [ ] **Step 1: Record the current vectors so you can prove the existing four did not change**

```bash
cd services/marketplace-api
sha256sum internal/handlers/platformadmin/testdata/vectors.json
python3 -c "import json;d=json.load(open('internal/handlers/platformadmin/testdata/vectors.json'));print([v['name'] for v in d]);print([v['signature'] for v in d])"
```

Save that output — Step 5 compares against it.

- [ ] **Step 2: Add the fifth input to the generator**

In `cmd/genvectors/main.go`, append one entry to the `inputs` slice, after the existing `query-value-with-space` entry, matching the surrounding style exactly:

```go
		{
			// Covers the FIVE sub-delimiter characters where url.QueryEscape
			// diverges from encodeURIComponent beyond the space: ! * ' ( ).
			// The other four vectors contain none of them, so a port that
			// applies only the documented %20 -> + fix passes all of them and
			// still 401s on a name like O'Brien (#372).
			name:          "query-value-with-sub-delims",
			requestTarget: "/api/v1/platform/admin/audit-logs?actor=O'Brien%20(ops)!&note=a%2Bb*",
			in: platformadmin.SignatureInput{
				Method: "GET", Path: "/api/v1/platform/admin/audit-logs",
				RawQuery:  "actor=O'Brien%20(ops)!&note=a%2Bb*",
				Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000005",
				Operator: "op_7f3a", Capability: "audit.read",
			},
		},
```

This exercises all six divergent characters in one vector: `'`, space, `(`, `)`, `!` in `actor`, and `*` plus a literal `+` (as `%2B`) in `note`.

- [ ] **Step 3: Regenerate the vectors file**

```bash
cd services/marketplace-api
go run ./internal/handlers/platformadmin/cmd/genvectors > \
  internal/handlers/platformadmin/testdata/vectors.json
```

Do not edit the file by hand afterwards.

- [ ] **Step 4: Inspect the generated canonical string and sanity-check it against the spec**

```bash
cd services/marketplace-api
python3 -c "import json;d=json.load(open('internal/handlers/platformadmin/testdata/vectors.json'));v=[x for x in d if x['name']=='query-value-with-sub-delims'][0];print(repr(v['canonical']))"
```

Expected: the canonical string's query fragment is `actor=O%27Brien+%28ops%29%21&note=a%2Bb%2A` — keys sorted, `'`→`%27`, space→`+`, `(`→`%28`, `)`→`%29`, `!`→`%21`, `*`→`%2A`, and the literal `+` preserved as `%2B`.

If what you observe differs, **do not adjust the expectation to match the output**. Report the difference — either the plan's expectation is wrong or something in the canonicaliser is not what finding 1 established, and both are worth knowing. (`actor` sorts before `note`, so no reordering should occur.)

- [ ] **Step 5: Prove the existing four vectors are byte-identical**

```bash
cd services/marketplace-api
python3 -c "import json;d=json.load(open('internal/handlers/platformadmin/testdata/vectors.json'));print([v['name'] for v in d]);print([v['signature'] for v in d])"
```

Expected: five entries; the **first four names and signatures are exactly what Step 1 recorded**. If any existing signature changed, STOP and escalate — that would mean a published vector broke, which is a blocker, not something to accept.

- [ ] **Step 6: Run the golden-vector test**

```bash
cd services/marketplace-api
go test ./internal/handlers/platformadmin/ -count=1 -run 'Vector|Signature' -v 2>&1 | tail -30
```

Expected: PASS, and the output shows a subtest for `query-value-with-sub-delims` — confirming the new vector is actually being exercised, not merely present in the file.

- [ ] **Step 7: MUTATION TEST — prove the new vector would catch the bug it exists to catch**

The vector's whole purpose is to fail for an implementation that escapes `!*'()` the `encodeURIComponent` way. Simulate that: in `testdata/vectors.json`, temporarily hand-edit **only the new vector's** `canonical` field, replacing `%27` with a literal `'`. Re-run Step 6's command.

Expected: **FAIL** on the `query-value-with-sub-delims` case with a canonical-string mismatch.

Then restore the file by regenerating it (Step 3's command) and re-run Step 6 to confirm PASS. If the mutated file still passed, the test is not comparing what it claims to and that is a blocker — escalate.

- [ ] **Step 8: Build, vet, full test**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && go test ./... -count=1
```

Expected: all clean; no new failures versus `main`.

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/cmd/genvectors/main.go \
        services/marketplace-api/internal/handlers/platformadmin/testdata/vectors.json
git commit -m "test(platformadmin): add a signature vector covering the ! * ' ( ) escaping divergence (#372)"
```

---

### Task 2: Correct the package doc to name all six characters

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/signature.go` (the `Query values are escaped...` bullet in the package doc, currently around lines 23-34)

**Interfaces:**
- Consumes: the vector name `query-value-with-sub-delims` produced by Task 1 — the doc references it, so Task 1 must land first.
- Produces: nothing later tasks depend on.

- [ ] **Step 1: Replace the escaping bullet**

In the package doc block at the top of `signature.go`, replace the bullet that begins `//   - Query values are escaped with application/x-www-form-urlencoded` (through the end of that bullet, ending `...what matters is how CanonicalQuery re-escapes it on the way out.`) with:

```go
//   - Query keys AND values are escaped with application/x-www-form-urlencoded
//     semantics (Go's url.QueryEscape). This diverges from
//     encodeURIComponent-style escaping (JavaScript, Python's
//     urllib.parse.quote) on SIX characters, not just the space:
//
//	    char   url.QueryEscape   encodeURIComponent
//	    space  +                 %20
//	    !      %21               !
//	    *      %2A               *
//	    '      %27               '
//	    (      %28               (
//	    )      %29               )
//
//     "~ - _ ." agree, and "+" is "%2B" on both sides. An implementation
//     built on encodeURIComponent must therefore convert "%20" to "+" (and
//     must not double-escape an existing "+") AND percent-escape the five
//     sub-delimiters "!*'()" — otherwise every query value containing one
//     silently 401s. An apostrophe in a person's name reaches this through
//     the audit-log actor filter, so this is not an edge case (#372).
//     See testdata vectors "query-value-with-space" and
//     "query-value-with-sub-delims". Only the *output* escaping diverges:
//     url.ParseQuery decodes both "%20" and "+" to a space on input, so the
//     raw query string the caller happened to build with is irrelevant —
//     what matters is how CanonicalQuery re-escapes it on the way out.
```

Note the table lines use a leading tab after `//` so gofmt renders them as a doc code block rather than reflowing them.

- [ ] **Step 2: Confirm gofmt is happy and the doc renders as intended**

```bash
cd services/marketplace-api
gofmt -l ./internal/handlers/platformadmin/
go doc ./internal/handlers/platformadmin 2>&1 | head -60
```

Expected: `gofmt -l` prints nothing (no files need formatting). `go doc` shows the table indented as a code block, not reflowed into a paragraph. If gofmt rewrites the block, accept gofmt's output — do not fight it.

- [ ] **Step 3: Build, vet, test**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./... && go test ./internal/handlers/platformadmin/ -count=1
```

Expected: all clean.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/signature.go
git commit -m "docs(platformadmin): name all six characters where QueryEscape diverges from encodeURIComponent (#372)"
```

---

## Self-Review

**Spec coverage.** #372 asks for two things: a fifth vector (Task 1, using a single vector covering all six characters rather than the issue's narrower suggestion) and a doc sentence naming `! * ' ( )` alongside the space (Task 2). Finding 3's extension — keys are escaped too — is folded into Task 2's wording. Finding 5 (no code change) is honoured: no production file is touched.

**Placeholder scan.** No TBDs. The vector input, the expected canonical fragment, and the full replacement doc block are all given literally.

**Type consistency.** The vector name `query-value-with-sub-delims` is identical in Task 1 Step 2, Task 1 Step 4, Task 1 Step 7 and Task 2 Step 1. `SignatureInput`'s field names match the four existing generator entries.

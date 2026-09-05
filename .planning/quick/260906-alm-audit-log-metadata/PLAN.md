# Emit audit-log metadata as the string the console actually reads

#313. `/admin/audit-logs` omits `metadata` from every row, so plan changes,
refund reasons, suspension reason codes and API-key revocations all reach the
platform console stripped of their detail.

The issue was filed as a question for the console team — object or string? —
and asked twice on #276 without an answer.

## It did not need an answer; it needed reading the consumer

`tesserix-home` is checked out. Three layers agree, independently:

- `platform-api/internal/modules/audit/internal/domain/entry.go:30` —
  `Metadata string`, and that struct IS the federation decode target
  (`service.go:104-125` unmarshals product responses straight into it)
- `apps/console/lib/audit.ts:196` — `optionalStr(row.metadata, ...)`, which
  `entry.go`'s own doc comment names as the real contract
- the console's own writer (`platform/audit/audit.go` `serialiseSummary`)
  renders compact JSON by hand into that column

So: a string, containing compact JSON. That also answers the "if string, how
do we flatten?" sub-question — not `k=v`, just the marshalled object.

## The stakes are higher than the issue assumed

#313 treats a wrong guess as a mis-rendered field. It is not.
`service.go:113` decodes a whole page with one `json.Unmarshal`. An object
where a string is expected fails that decode, the closure returns an error,
and mark8ly is recorded as a federation FAILURE — **every** mark8ly audit row
disappears from the console, not just this field.

Which is also why the current omission was the right holding position.

## Tasks

1. `metadata` on `auditRow`, `omitempty`.
2. `metadataOf` in `toRow`: `json.Marshal` the jsonb map; "" for an empty or
   nil map, and "" on a marshal error so one bad row cannot cost the page.
3. Golden fixture: one row with metadata, one without, pinning both the
   encoding and the omission.

## Done when

- A row with metadata carries a JSON *string*; the raw bytes are quoted.
- Empty and nil metadata omit the key entirely — never `"{}"`.
- Output is byte-stable across calls, so a poller sees no phantom change.
- The pinned-contract golden test covers both cases.

## Flagged, not decided here

`metadata` carries `customer_email` on two paths (`abandoned_cart_service.go`,
`checkout_ext.go`). This ships it to the console. That is an HMAC-authenticated
operator surface which already receives merchant emails in `actor`, so it is
consistent rather than new — but it is a widening of what leaves this service
and belongs in the PR description, not in a silent commit.

# Metro image-size compatibility

Metro in the existing Expo SDKs calls `require('image-size')` as a function,
with either bytes or a synchronous file path. `image-size@2` fixes
GHSA-w3rx-r6r6-pgpr and GHSA-5p2g-fcmc-qvqq but removes these v1 entry points.

This adapter keeps those entry points and the callback/format control APIs,
while delegating all image parsing to the patched upstream package through
the `safe-image-size` npm alias. Synchronous file reads retain v1's 512 KiB
limit. No vulnerable v1 parser is included.

The root override applies this adapter to all existing consumers. Remove it
once every Metro/Expo consumer supports the upstream v2 API directly. Run
`node --test vendor/image-size/security.test.cjs` to verify both API forms
and rejection of an ICNS entry that previously caused an infinite loop.

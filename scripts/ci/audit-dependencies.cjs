// Runs `npm audit` at any severity, tolerating only the advisories listed below.
// A vulnerability passes only when EVERY advisory it stems from is allowed, so a
// package picking up an unrelated advisory still fails. Same rule as the shared
// workflow's production audit gate; keep this list in step with audit_allowlist
// in .github/workflows/ci.yml.
const { execFileSync } = require('node:child_process');

const ALLOWLIST = new Set([
  // picomatch ReDoS via extglobs; no patched release (<= 4.0.7 affected).
  // Build-time globs only. Remove once picomatch ships a fix.
  'GHSA-fc5q-39xc-vjpf',
]);

function advisoryIds(name, vulns, seen = new Set()) {
  if (seen.has(name)) return new Set();
  seen.add(name);
  const ids = new Set();
  for (const via of vulns[name]?.via ?? []) {
    if (typeof via === 'string') {
      for (const id of advisoryIds(via, vulns, seen)) ids.add(id);
    } else {
      const id = String(via.url ?? '').split('/').pop();
      // An advisory we cannot identify can never be allowed.
      ids.add(id || `unknown:${name}`);
    }
  }
  return ids;
}

function blocking(report, allowlist = ALLOWLIST) {
  const vulns = report.vulnerabilities ?? {};
  return Object.keys(vulns).filter((name) => {
    const ids = advisoryIds(name, vulns);
    return ids.size === 0 || [...ids].some((id) => !allowlist.has(id));
  });
}

module.exports = { blocking };

if (require.main === module) {
  let out;
  try {
    out = execFileSync('npm', ['audit', '--json'], { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 });
  } catch (err) {
    // npm audit exits non-zero whenever it finds anything; the gate below decides.
    out = err.stdout;
  }
  const report = JSON.parse(out);
  if (report.error) {
    console.error('npm audit failed:', report.error.summary ?? report.error);
    process.exit(1);
  }
  const bad = blocking(report);
  if (bad.length) {
    console.error(`Vulnerable packages not covered by the allowlist: ${bad.join(', ')}`);
    console.error('Run `npm audit` for details.');
    process.exit(1);
  }
  console.log(`npm audit: no vulnerabilities outside the allowlist (${[...ALLOWLIST].join(', ')}).`);
}

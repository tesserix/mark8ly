const { test } = require('node:test');
const assert = require('node:assert/strict');
const { blocking } = require('./audit-dependencies.cjs');

const advisory = (id) => ({ url: `https://github.com/advisories/${id}` });
const allow = new Set(['GHSA-aaaa-bbbb-cccc']);

test('passes packages whose only advisories are allowed, directly or transitively', () => {
  const report = {
    vulnerabilities: {
      picomatch: { via: [advisory('GHSA-aaaa-bbbb-cccc')] },
      anymatch: { via: ['picomatch'] },
    },
  };
  assert.deepEqual(blocking(report, allow), []);
});

test('fails a package that also has an advisory outside the allowlist', () => {
  const report = {
    vulnerabilities: {
      picomatch: { via: [advisory('GHSA-aaaa-bbbb-cccc'), advisory('GHSA-dddd-eeee-ffff')] },
      anymatch: { via: ['picomatch'] },
    },
  };
  assert.deepEqual(blocking(report, allow), ['picomatch', 'anymatch']);
});

test('fails closed on advisories it cannot attribute', () => {
  const report = { vulnerabilities: { x: { via: [] }, y: { via: [{}] } } };
  assert.deepEqual(blocking(report, allow), ['x', 'y']);
});

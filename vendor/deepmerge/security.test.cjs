const { test } = require('node:test');
const assert = require('node:assert/strict');
const merge = require('./index.cjs');

test('untrusted target keys cannot poison the returned prototype', () => {
  const target = JSON.parse('{"__proto__":{"isAdmin":true},"name":"store"}');
  const result = merge(target, { enabled: true });
  assert.equal(Object.getPrototypeOf(result), Object.prototype);
  assert.equal(result.isAdmin, undefined);
  assert.equal(Object.prototype.isAdmin, undefined);
  assert.equal(result.name, 'store');
  assert.equal(result.enabled, true);
});

test('null-prototype targets cannot bypass source key validation', () => {
  const result = merge(Object.create(null), JSON.parse('{"__proto__":{"isAdmin":true}}'));
  assert.equal(Object.getPrototypeOf(result), Object.prototype);
  assert.equal(result.isAdmin, undefined);
});

test('nested objects and array elements cannot poison returned prototypes', () => {
  const payload = JSON.parse('{"nested":{"__proto__":{"isAdmin":true}},"items":[{"__proto__":{"isAdmin":true}}]}');
  const result = merge(payload, {});
  assert.equal(Object.getPrototypeOf(result.nested), Object.prototype);
  assert.equal(Object.getPrototypeOf(result.items[0]), Object.prototype);
});

test('preserves the Expo and Jest merge API without mutating inputs', () => {
  const symbol = Symbol('config');
  const target = { nested: { first: 1 }, items: [{ a: 1 }], [symbol]: { a: 1 } };
  const source = { nested: { second: 2 }, items: [{ b: 2 }], [symbol]: { b: 2 } };
  const result = merge(target, source);
  assert.deepEqual(result.nested, { first: 1, second: 2 });
  assert.deepEqual(result.items, [{ a: 1 }, { b: 2 }]);
  assert.deepEqual(result[symbol], { a: 1, b: 2 });
  assert.deepEqual(target.nested, { first: 1 });
  assert.notEqual(result.items[0], target.items[0]);
  assert.deepEqual(merge.all([{ a: 1 }, { b: 2 }]), { a: 1, b: 2 });
  assert.deepEqual(merge(target, source, { arrayMerge: (_, incoming) => incoming }).items, source.items);
  assert.deepEqual(merge({ n: { a: 1 } }, { n: { b: 2 } }, {
    customMerge: key => key === 'n' ? () => 'custom' : undefined,
  }), { n: 'custom' });
});

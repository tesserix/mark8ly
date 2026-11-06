const { test } = require('node:test');
const assert = require('node:assert/strict');
const decode = require('./index.cjs');

test('keeps the callable CommonJS URI decoding API', () => {
  assert.equal(decode('hello%20world'), 'hello world');
  assert.equal(decode('%E2%9C%93'), '✓');
  assert.equal(decode('plain'), 'plain');
  assert.throws(() => decode(null), TypeError);
});

test('legacy query-string consumers can decode navigation parameters', () => {
  const query = require('query-string');
  assert.deepEqual({ ...query.parse('name=hello%20world&item=%E2%9C%93') }, {
    item: '✓',
    name: 'hello world',
  });
});

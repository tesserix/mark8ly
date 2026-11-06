const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const sizeOf = require('./index.cjs');

const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aPuoAAAAASUVORK5CYII=', 'base64');

test('Metro can read dimensions through the callable CommonJS buffer API', () => {
  assert.equal(sizeOf(png).width, 1);
  assert.equal(sizeOf(png).height, 1);
  assert.equal(sizeOf.imageSize, sizeOf);
  assert.equal(sizeOf.default, sizeOf);
});

test('Metro can read file paths synchronously and legacy callbacks still work', async t => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'image-size-test-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const file = path.join(dir, 'pixel.png');
  fs.writeFileSync(file, png);
  assert.equal(sizeOf(file).width, 1);
  const dimensions = await new Promise((resolve, reject) => {
    sizeOf(file, (error, value) => error ? reject(error) : resolve(value));
  });
  assert.equal(dimensions.height, 1);
  assert.throws(() => sizeOf(path.join(dir, 'missing.png')), { code: 'ENOENT' });
  fs.writeFileSync(file, '');
  assert.throws(() => sizeOf(file), /Empty file/);
});

test('filesystem and format controls retain their behavior', () => {
  sizeOf.disableFS(true);
  try {
    assert.throws(() => sizeOf('/unused.png'), /Uint8Array/);
    assert.equal(sizeOf(png).width, 1);
  } finally {
    sizeOf.disableFS(false);
  }
  sizeOf.disableTypes(['png']);
  try {
    assert.throws(() => sizeOf(png), /disabled file type/);
  } finally {
    sizeOf.disableTypes([]);
  }
});

test('zero-length ICNS entries reject instead of looping indefinitely', () => {
  const script = `
    const sizeOf = require(${JSON.stringify(path.join(__dirname, 'index.cjs'))});
    const data = Buffer.alloc(16);
    data.write('icns'); data.writeUInt32BE(16, 4); data.write('ic07', 8);
    require('node:assert/strict').throws(() => sizeOf(data), /Invalid ICNS/);
  `;
  const result = spawnSync(process.execPath, ['-e', script], { timeout: 3000 });
  assert.equal(result.error, undefined);
  assert.equal(result.status, 0, result.stderr?.toString());
});

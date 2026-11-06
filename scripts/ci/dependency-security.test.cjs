const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createRequire } = require('node:module');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

const app = path.resolve(__dirname, '../../apps/mobile-admin');
const expo = createRequire(require.resolve('expo/package.json', { paths: [app] }));

test('Expo configuration loads the patched merge implementation', () => {
  const config = createRequire(expo.resolve('@expo/config'));
  const merge = config('deepmerge');
  const result = merge(JSON.parse('{"__proto__":{"isAdmin":true}}'), { name: 'store' });
  assert.equal(result.isAdmin, undefined);
  assert.equal(result.name, 'store');
  assert.equal(Object.getPrototypeOf(result), Object.prototype);
});

test('Expo dev tools validate plugins with the patched Zod version', () => {
  const cli = createRequire(expo.resolve('@expo/cli/package.json'));
  const { PluginSchema } = cli('./build/src/start/server/DevToolsPlugin.schema.js');
  const plugin = {
    packageName: 'test-plugin',
    packageRoot: '/tmp/test-plugin',
    cliExtensions: {
      description: 'Test',
      entryPoint: 'index.js',
      commands: [{ name: 'test', title: 'Test', environments: ['cli'] }],
    },
  };
  assert.equal(PluginSchema.safeParse(plugin).success, true);
  assert.equal(PluginSchema.safeParse({ ...plugin, packageName: '' }).success, false);
});

test('Metro builds image asset metadata using the compatible patched parser', async t => {
  const metro = createRequire(require.resolve('metro/package.json', { paths: [app] }));
  const { getAssetData } = metro('./src/Assets.js');
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'metro-security-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const file = path.join(dir, 'pixel.png');
  fs.writeFileSync(file, Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aPuoAAAAASUVORK5CYII=', 'base64'));
  const asset = await getAssetData(file, 'pixel.png', [], null, '/assets');
  assert.equal(asset.width, 1);
  assert.equal(asset.height, 1);
  assert.equal(asset.type, 'png');
});

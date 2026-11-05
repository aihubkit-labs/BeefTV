import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const repo = fileURLToPath(new URL('../../', import.meta.url));

function fixture(t) {
  const root = mkdtempSync(path.join(tmpdir(), 'aihubkit build with spaces '));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const directory of ['scripts/aihubkit', 'backend', 'web', 'bin', 'runtime/bin', 'plugin-packages/current/docs']) {
    mkdirSync(path.join(root, directory), { recursive: true });
  }
  const put = (name, source) => writeFileSync(path.join(root, name), source, { mode: 0o755 });
  copyFileSync(path.join(repo, 'scripts/aihubkit/update-local-app.sh'), path.join(root, 'scripts/aihubkit/update-local-app.sh'));
  for (const name of ['build.py', 'brands.py']) {
    copyFileSync(path.join(repo, 'scripts/aihubkit', name), path.join(root, 'scripts/aihubkit', name));
  }
  for (const name of ['build-packages.sh', 'embed-documentation.mjs']) {
    copyFileSync(path.join(repo, 'plugin-packages', name), path.join(root, 'plugin-packages', name));
  }
  symlinkSync(process.execPath, path.join(root, 'runtime/bin/node'));
  put('bin/uname', '#!/bin/sh\nif [ "$1" = -s ]; then echo Darwin; else echo arm64; fi\n');
  put('bin/xcrun', '#!/bin/sh\necho /usr/bin/clang\n');
  put('bin/codesign', '#!/bin/sh\nexit 0\n');
  put('bin/go', `#!/bin/sh
set -eu
if [ "$1" = version ]; then echo "go version fixture"; exit 0; fi
printf 'dependencies:go:%s\\n' "$GOTOOLCHAIN" >> "$BUILD_TEST_LOG"
[ "$BUILD_TEST_FAIL" != go ]
`);
  put('bin/bun', `#!/bin/sh
set -eu
if [ "$1" != install ]; then exec "$BUILD_TEST_NODE" "$@"; fi
[ "$2" = --frozen-lockfile ]
printf 'dependencies:web\\n' >> "$BUILD_TEST_LOG"
[ "$BUILD_TEST_FAIL" != dependencies ]
mkdir -p node_modules
`);
  put('scripts/package-agent-host.mjs', `
import { appendFileSync } from 'node:fs';
appendFileSync(process.env.BUILD_TEST_LOG, 'toolchains\\n');
if (process.env.BUILD_TEST_FAIL === 'runtime') process.exit(1);
`);
  put('scripts/update-local-beeftv-app.sh', `#!/bin/sh
set -eu
[ "$BEEFTV_SKIP_LOCAL_VERIFY" = 0 ]
[ "$CGO_ENABLED" = 1 ]
printf 'update:%s\\n' "$BEEFTV_INTEGRATION" >> "$BUILD_TEST_LOG"
[ "$BUILD_TEST_FAIL" != update ]
`);
  put('plugin-packages/current/manifest.json', JSON.stringify({ apiVersion: 'beeftv.plugin/v2', id: 'current', version: '1.0.0' }));
  put('plugin-packages/current/README.md', '# Current plugin\n');
  put('plugin-packages/current/docs/interface.md', '# Current contract\n');
  const env = {
    ...process.env,
    PATH: `${path.join(root, 'bin')}:${process.env.PATH}`,
    BEEFTV_NODE_RUNTIME: path.join(root, 'runtime'),
    BEEFTV_SKIP_LOCAL_VERIFY: '1',
    BUILD_TEST_LOG: path.join(root, 'steps.log'),
    BUILD_TEST_NODE: process.execPath,
  };
  for (const name of ['BEEFTV_GO_DIR', 'BEEFTV_WAILS_PLATFORM', 'GOTOOLCHAIN', 'BUILD_TEST_FAIL', 'BEEFTV_INTEGRATION']) delete env[name];
  env.BUILD_TEST_FAIL = '';
  env.BEEFTV_INTEGRATION = 'unset';
  const run = (overrides = {}) => spawnSync('/bin/bash', [path.join(root, 'scripts/aihubkit/update-local-app.sh')], {
    cwd: tmpdir(), env: { ...env, ...overrides }, encoding: 'utf8', timeout: 30_000,
  });
  const steps = () => existsSync(env.BUILD_TEST_LOG) ? readFileSync(env.BUILD_TEST_LOG, 'utf8').trim().split('\n') : [];
  return { root, put, run, steps };
}

test('fresh checkout installs dependencies and generates current plugins before calling the official updater', (t) => {
  const { root, put, run, steps } = fixture(t);
  const source = path.join(root, 'plugin-packages/current/manifest.json');
  const original = readFileSync(source, 'utf8');
  put('plugin-packages/keep.txt', 'unrelated file');

  const result = run();
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.deepEqual(steps(), ['toolchains', 'dependencies:go:auto', 'dependencies:web', 'update:unset']);
  assert.ok(existsSync(path.join(root, 'web/node_modules')));
  assert.equal(readFileSync(source, 'utf8'), original);
  assert.equal(readFileSync(path.join(root, 'plugin-packages/keep.txt'), 'utf8'), 'unrelated file');
  assert.deepEqual(readdirSync(path.join(root, '.local/cache')), []);
  const archive = path.join(root, 'plugin-packages/current.beeftv-plugin');
  const manifest = spawnSync('unzip', ['-p', archive, 'manifest.json'], { encoding: 'utf8' });
  assert.equal(manifest.status, 0, manifest.stderr);
  assert.match(JSON.parse(manifest.stdout).documentation, /Current contract/);

  put('plugin-packages/removed.beeftv-plugin', 'obsolete archive');
  put('plugin-packages/current.beeftv-plugin', 'stale archive');
  put('plugin-packages/current/docs/interface.md', '# Updated contract\n');
  const rebuilt = run();
  assert.equal(rebuilt.status, 0, rebuilt.stdout + rebuilt.stderr);
  assert.equal(existsSync(path.join(root, 'plugin-packages/removed.beeftv-plugin')), false);
  assert.equal(readFileSync(source, 'utf8'), original);
  assert.equal(readFileSync(path.join(root, 'plugin-packages/keep.txt'), 'utf8'), 'unrelated file');
  const updated = spawnSync('unzip', ['-p', archive, 'manifest.json'], { encoding: 'utf8' });
  assert.match(JSON.parse(updated.stdout).documentation, /Updated contract/);
});

for (const failure of ['runtime', 'go', 'dependencies', 'plugins']) {
  test(`${failure} failure stops before installation and preserves existing plugin output`, (t) => {
    const { root, put, run, steps } = fixture(t);
    put('plugin-packages/current.beeftv-plugin', 'previous output');
    if (failure === 'plugins') put('plugin-packages/current/manifest.json', 'invalid JSON');
    const result = run({ BUILD_TEST_FAIL: failure });
    assert.notEqual(result.status, 0);
    assert.equal(steps().some((step) => step.startsWith('update:')), false);
    assert.equal(readFileSync(path.join(root, 'plugin-packages/current.beeftv-plugin'), 'utf8'), 'previous output');
    const cache = path.join(root, '.local/cache');
    if (existsSync(cache)) assert.deepEqual(readdirSync(cache), []);
  });
}

test('an incomplete package set cannot reuse old archives', (t) => {
  const { put, run, steps } = fixture(t);
  put('plugin-packages/build-packages.sh', '#!/bin/sh\nexit 0\n');
  put('plugin-packages/current.beeftv-plugin', 'previous output');
  const result = run();
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /generation is incomplete/);
  assert.equal(steps().some((step) => step.startsWith('update:')), false);
});

test('official local mode and explicit toolchain policy reach the updater unchanged', (t) => {
  const { run, steps } = fixture(t);
  const result = run({ BEEFTV_INTEGRATION: 'local', GOTOOLCHAIN: 'local' });
  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.deepEqual(steps(), ['toolchains', 'dependencies:go:local', 'dependencies:web', 'update:local']);
});

test('a cross-architecture target is rejected before preparing or installing anything', (t) => {
  const { run, steps } = fixture(t);
  const result = run({ BEEFTV_WAILS_PLATFORM: 'darwin/amd64' });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /requires the host target/);
  assert.deepEqual(steps(), []);
});

test('updater failure propagates and the staging directory is cleaned', (t) => {
  const { root, run } = fixture(t);
  const result = run({ BUILD_TEST_FAIL: 'update' });
  assert.notEqual(result.status, 0);
  assert.deepEqual(readdirSync(path.join(root, '.local/cache')), []);
});

const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { test } = require('node:test');
const { bumpVersion } = require('./bump-version.cjs');
const { findCodeChanges } = require('./has-code-changes.cjs');

const metadata = [
  'build/config.yml', 'build/windows/info.json', 'build/windows/nsis/wails_tools.nsh',
  'build/linux/nfpm/nfpm.yaml', 'frontend/package.json', 'frontend/package-lock.json',
];

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'depmanager-release-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const write = (file, content) => {
    fs.mkdirSync(path.dirname(path.join(root, file)), { recursive: true });
    fs.writeFileSync(path.join(root, file), content);
  };
  for (const file of metadata) write(file, fs.readFileSync(path.join(__dirname, '..', file)));
  write('app.go', 'package main\n');
  write('frontend/src/App.tsx', 'export default function App() { return null }\n');
  write('README.md', '# DepManager\n');
  const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  git('init', '-q');
  git('config', 'user.name', 'Release test');
  git('config', 'user.email', 'release-test@example.com');
  const commit = () => { git('add', '.'); git('commit', '-qm', 'fixture'); return git('rev-parse', 'HEAD'); };
  const base = commit();
  const read = (file) => fs.readFileSync(path.join(root, file), 'utf8');
  const json = (file) => JSON.parse(read(file));
  const numeric = read('build/config.yml').match(/^  version: "([^"]+)"/m)[1];
  const parts = numeric.split('.').map(Number);
  const next = parts[0] + '.' + parts[1] + '.' + (parts[2] + 1);
  return { root, write, read, json, commit, base, next };
}

test('nightly app, Debian, Windows and NSIS versions stay compatible', (t) => {
  const f = fixture(t);
  const version = bumpVersion(f.root, 42);
  assert.equal(version, f.next + '-nightly.42');
  assert.equal(f.json('frontend/package.json').version, version);
  assert.equal(f.json('frontend/package-lock.json').packages[''].version, version);
  assert.equal(f.json('build/windows/info.json').fixed.file_version, f.next);
  assert.equal(f.json('build/windows/info.json').info['0000'].ProductVersion, version);
  assert.ok(f.read('build/config.yml').includes('version: "' + f.next + '"'));
  assert.ok(f.read('build/linux/nfpm/nfpm.yaml').includes('version: "' + version + '"'));
  assert.ok(f.read('build/windows/nsis/wails_tools.nsh').includes('INFO_PRODUCTVERSION "' + f.next + '"'));
  const following = bumpVersion(f.root, 43);
  assert.notEqual(following, version);
  assert.ok(following.endsWith('-nightly.43'));
});

test('stable version bump remains available', (t) => {
  const f = fixture(t);
  assert.equal(bumpVersion(f.root), f.next);
});

test('invalid nightly numbers cannot change metadata', (t) => {
  const f = fixture(t);
  const before = f.read('frontend/package.json');
  for (const number of ['0', '-1', '01', '1.2', 'no']) {
    assert.throws(() => bumpVersion(f.root, number), /positive integer/);
  }
  assert.equal(f.read('frontend/package.json'), before);
});

test('malformed packaging metadata fails before any version is written', (t) => {
  const f = fixture(t);
  const before = f.read('frontend/package.json');
  f.write('build/linux/nfpm/nfpm.yaml', 'name: depmanager\n');
  assert.throws(() => bumpVersion(f.root, 42), /package version/);
  assert.equal(f.read('frontend/package.json'), before);
});

test('unchanged source skips publication', (t) => {
  const f = fixture(t);
  assert.deepEqual(findCodeChanges(f.root, f.base), []);
});

test('documentation and release version metadata do not create another build', (t) => {
  const f = fixture(t);
  f.write('README.md', '# Updated documentation\n');
  bumpVersion(f.root, 42);
  f.commit();
  assert.deepEqual(findCodeChanges(f.root, f.base), []);
});

test('backend, frontend, dependencies and assets trigger publication', (t) => {
  const f = fixture(t);
  f.write('app.go', 'package main\n// new behavior\n');
  f.write('frontend/src/App.tsx', 'export default function App() { return "updated" }\n');
  const pkg = f.json('frontend/package.json');
  pkg.dependencies.react = '^19.0.0';
  f.write('frontend/package.json', JSON.stringify(pkg));
  f.write('build/appicon.png', Buffer.from([1, 2, 3]));
  f.commit();
  assert.deepEqual(findCodeChanges(f.root, f.base).sort(), [
    'app.go', 'build/appicon.png', 'frontend/package.json', 'frontend/src/App.tsx',
  ].sort());
});

test('lockfile changes trigger publication even with unchanged version metadata', (t) => {
  const f = fixture(t);
  const lock = f.json('frontend/package-lock.json');
  lock.packages['node_modules/react'].version = '99.0.0';
  f.write('frontend/package-lock.json', JSON.stringify(lock));
  f.commit();
  assert.deepEqual(findCodeChanges(f.root, f.base), ['frontend/package-lock.json']);
});

test('deleted source and changed build recipes trigger publication', (t) => {
  const f = fixture(t);
  fs.unlinkSync(path.join(f.root, 'frontend/src/App.tsx'));
  f.write('build/linux/nfpm/nfpm.yaml', f.read('build/linux/nfpm/nfpm.yaml') + '\nreplaces:\n  - old-package\n');
  f.commit();
  assert.deepEqual(findCodeChanges(f.root, f.base).sort(), ['build/linux/nfpm/nfpm.yaml', 'frontend/src/App.tsx']);
});

test('failed builds remain eligible when compared with the last published version', (t) => {
  const f = fixture(t);
  f.write('app.go', 'package main\n// new behavior\n');
  bumpVersion(f.root, 42);
  const failedBuild = f.commit();
  assert.deepEqual(findCodeChanges(f.root, f.base, failedBuild), ['app.go']);
  assert.deepEqual(findCodeChanges(f.root, failedBuild), []);
});

test('a missing first baseline builds once and invalid baselines fail closed', (t) => {
  const f = fixture(t);
  assert.ok(findCodeChanges(f.root, '').length > 0);
  assert.throws(() => findCodeChanges(f.root, 'invalid-release-tag'));
});

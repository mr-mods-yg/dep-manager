const assert = require('node:assert/strict');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { test } = require('node:test');
const { bumpVersion, nextReleaseVersion } = require('./bump-version.cjs');
const { parseVersion, compareVersions } = require('./semver.cjs');
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
  write('updates/release-plan.json', JSON.stringify({ version: '1.0.0' }));
  // Exercise the same starting version on every release branch.
  write('build/config.yml', fs.readFileSync(path.join(root, 'build/config.yml'), 'utf8').replace(/^  version: "[^"]+" # The application version$/m, '  version: "0.0.4" # The application version'));
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
  return { root, write, read, json, commit, base, git };
}

test('nightly app, Debian, Windows and NSIS versions stay compatible', (t) => {
  const f = fixture(t);
  const version = bumpVersion(f.root);
  assert.equal(version, '1.0.0-alpha.1');
  assert.equal(f.json('frontend/package.json').version, version);
  assert.equal(f.json('frontend/package-lock.json').packages[''].version, version);
  assert.equal(f.json('frontend/package.json').releaseChannel, 'nightly');
  assert.equal(f.json('build/windows/info.json').fixed.file_version, '1.0.0');
  assert.equal(f.json('build/windows/info.json').info['0000'].ProductVersion, version);
  assert.ok(f.read('build/config.yml').includes('version: "1.0.0"'));
  assert.ok(f.read('build/linux/nfpm/nfpm.yaml').includes('version: "' + version + '"'));
  assert.ok(f.read('build/windows/nsis/wails_tools.nsh').includes('INFO_PRODUCTVERSION "1.0.0"'));
  f.git('tag', 'v' + version);
  assert.equal(bumpVersion(f.root), '1.0.0-alpha.2');
});

test('stable removes the prerelease suffix from the planned target', (t) => {
  const f = fixture(t);
  assert.equal(bumpVersion(f.root, { channel: 'stable' }), '1.0.0');
  assert.equal(f.json('frontend/package.json').releaseChannel, 'stable');
});

test('invalid targets and channels cannot change metadata', (t) => {
  const f = fixture(t);
  const before = f.read('frontend/package.json');
  for (const version of ['01.0.0', '1.2', 'no', '1.0.0-alpha.1', '1.0.0+build', '65536.0.0']) {
    f.write('updates/release-plan.json', JSON.stringify({ version }));
    assert.throws(() => bumpVersion(f.root));
  }
  f.write('updates/release-plan.json', '{"version":"1.0.0"}');
  assert.throws(() => bumpVersion(f.root, { channel: 'invalid' }), /channel/);
  assert.equal(f.read('frontend/package.json'), before);
});

test('malformed packaging metadata fails before any version is written', (t) => {
  const f = fixture(t);
  const before = f.read('frontend/package.json');
  f.write('build/linux/nfpm/nfpm.yaml', 'name: depmanager\n');
  assert.throws(() => bumpVersion(f.root), /package version/);
  assert.equal(f.read('frontend/package.json'), before);
});

test('unchanged source skips publication', (t) => {
  const f = fixture(t);
  assert.deepEqual(findCodeChanges(f.root, f.base), []);
});

test('documentation and release version metadata do not create another build', (t) => {
  const f = fixture(t);
  f.write('README.md', '# Updated documentation\n');
  bumpVersion(f.root);
  f.write('updates/release-plan.json', '{"version":"2.0.0"}');
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
  bumpVersion(f.root);
  const failedBuild = f.commit();
  assert.deepEqual(findCodeChanges(f.root, f.base, failedBuild), ['app.go']);
  assert.deepEqual(findCodeChanges(f.root, failedBuild), []);
});

test('a missing first baseline builds once and invalid baselines fail closed', (t) => {
  const f = fixture(t);
  assert.ok(findCodeChanges(f.root, '').length > 0);
  assert.throws(() => findCodeChanges(f.root, 'invalid-release-tag'));
});

test('nightly, beta and stable use one target with independent counters', () => {
  const tags = ['v1.3.0-alpha.1', 'v1.3.0-alpha.9', 'v1.3.0-beta.1', 'v1.2.1-alpha.99', 'v1.3.0-alpha.01'];
  assert.equal(nextReleaseVersion('1.3.0', 'nightly', tags, ['v1.2.0']), '1.3.0-alpha.10');
  assert.equal(nextReleaseVersion('1.3.0', 'beta', tags, ['v1.2.0']), '1.3.0-beta.2');
  assert.equal(nextReleaseVersion('1.3.0', 'stable', tags, ['v1.2.0']), '1.3.0');
  assert.equal(nextReleaseVersion('1.3.1', 'nightly', tags, ['v1.3.0']), '1.3.1-alpha.1');
  assert.equal(nextReleaseVersion('2.0.0', 'beta', tags, ['v1.3.0']), '2.0.0-beta.1');
});

test('published stable versions and reserved tags cannot be reused', () => {
  for (const channel of ['nightly', 'beta', 'stable']) {
    for (const target of ['1.2.0', '1.1.9']) {
      assert.throws(() => nextReleaseVersion(target, channel, [], ['v1.2.0']), /newer than published stable/);
    }
    assert.throws(() => nextReleaseVersion('1.3.0', channel, ['v1.3.0']), /already has a tag/);
  }
});

test('failed tagged builds reserve their counter without using workflow run numbers', () => {
  assert.equal(nextReleaseVersion('1.0.0', 'nightly', ['v1.0.0-alpha.1']), '1.0.0-alpha.2');
  assert.equal(nextReleaseVersion('1.0.0', 'beta', ['v1.0.0-alpha.8']), '1.0.0-beta.1');
});

test('SemVer precedence follows the specification and ignores build metadata', () => {
  const examples = ['1.0.0-alpha', '1.0.0-alpha.1', '1.0.0-alpha.beta', '1.0.0-beta', '1.0.0-beta.2', '1.0.0-beta.11', '1.0.0-rc.1', '1.0.0', '2.0.0', '2.1.0', '2.1.1'];
  for (let i = 1; i < examples.length; i++) assert.equal(compareVersions(examples[i - 1], examples[i]), -1);
  assert.equal(compareVersions('1.0.0+build.123', '1.0.0+other.456'), 0);
  assert.equal(compareVersions('1.0.0-alpha.9007199254740993', '1.0.0-alpha.9007199254740992'), 1);
  assert.equal(compareVersions('1.0.0-alpha.2', '1.0.0-alpha.beta'), -1);
  assert.equal(compareVersions('1.0.0-alpha.2', '1.0.0-alpha.2'), 0);
  for (const version of ['v1.0.0', '01.0.0', '1.0.0-alpha.01', '1.0.0-', '1.0.0+', '1.0.0+bad_']) {
    assert.throws(() => parseVersion(version), /Invalid SemVer/);
  }
});

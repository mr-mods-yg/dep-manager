const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { test } = require('node:test');
const { releaseNotes, recordRelease } = require('./changelog.cjs');
const { findCodeChanges } = require('./has-code-changes.cjs');

const source = '# Changelog\n\n## [Unreleased]\n\n### Added\n\n- A new feature.\n  More information about this feature.\n\n### Fixed\n\n- A bug fix with [details].\n\n## [0.0.5-nightly.5] - 2026-10-14\n\n### Fixed\n\n- Previous fix.\n\n[unreleased]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.5-nightly.5...nightly\n[0.0.5-nightly.5]: https://github.com/mr-mods-yg/dep-manager/releases/tag/v0.0.5-nightly.5\n[details]: https://example.com/details\n';
const options = {
  version: '0.0.6-nightly.8', date: '2026-10-15', branch: 'nightly',
  notes: releaseNotes(source),
};

test('extract human-written notes with multiline entries and reference links', () => {
  const notes = releaseNotes(source.replace(/\n/g, '\r\n'));
  assert.ok(notes.startsWith('### Added\n\n- A new feature.\n  More information'));
  assert.ok(notes.includes('### Fixed\n\n- A bug fix with [details].'));
  assert.ok(notes.includes('[details]: https://example.com/details'));
  assert.ok(!notes.includes('Previous fix.'));
});

test('record a dated release above older history and refresh comparison links', () => {
  const recorded = recordRelease(source, options);
  assert.ok(recorded.includes('## [Unreleased]\n\n## [0.0.6-nightly.8] - 2026-10-15'));
  assert.ok(recorded.indexOf('## [0.0.6-nightly.8]') < recorded.indexOf('## [0.0.5-nightly.5]'));
  assert.ok(recorded.includes('- Previous fix.'));
  assert.ok(recorded.includes('[Unreleased]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.6-nightly.8...nightly'));
  assert.ok(recorded.includes('[0.0.6-nightly.8]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.5-nightly.5...v0.0.6-nightly.8'));
  assert.equal(recorded.match(/^\[unreleased\]:/gmi).length, 1);
});

test('retain changes added while the published build was running', () => {
  const concurrent = source.replace('### Added\n\n', '### Added\n\n- A later feature.\n\n');
  const recorded = recordRelease(concurrent, options);
  const pending = releaseNotes(recorded);
  assert.ok(pending.includes('- A later feature.'));
  assert.ok(!pending.includes('- A new feature.'));
  assert.ok(!pending.includes('### Fixed'));
  assert.ok(releaseNotes(recorded, options.version).includes('A new feature.'));
});

test('preserve edited entries for the next release while publishing the snapshot', () => {
  const concurrent = source.replace('- A new feature.', '- An expanded feature.');
  const recorded = recordRelease(concurrent, options);
  assert.ok(releaseNotes(recorded).includes('An expanded feature.'));
  assert.ok(releaseNotes(recorded, options.version).includes('A new feature.'));
});

test('recording a release twice preserves the curated record', () => {
  const recorded = recordRelease(source, options);
  assert.equal(recordRelease(recorded, options), recorded);
  const edited = recorded.replace('### Added\n\n- A new feature.', '### Added\n\n- A clarified feature.');
  assert.equal(recordRelease(edited, options), edited);
});

test('first release works without older history and empty categories are omitted', () => {
  const first = '# Changelog\n\n## [Unreleased]\n\n### Added\n\n- Initial feature.\n\n### Fixed\n';
  const recorded = recordRelease(first, { ...options, notes: releaseNotes(first), branch: 'main' });
  assert.ok(recorded.includes('/releases/tag/v0.0.6-nightly.8'));
  assert.ok(recorded.includes('...main'));
  assert.ok(!recorded.includes('### Fixed'));
  assert.ok(!recorded.includes('\n\n\n'));
});

test('keep referenced snapshot links even if later changes removed the definition', () => {
  const recorded = recordRelease(source.replace('[details]: https://example.com/details\n', ''), options);
  assert.ok(recorded.includes('[details]: https://example.com/details'));
});

test('reject empty notes and malformed changelog categories', () => {
  assert.throws(() => releaseNotes('# Changelog\n\n## [Unreleased]\n'), /Add curated changes/);
  assert.throws(() => releaseNotes(source.replace('### Added', '### Updates')), /Invalid or duplicate/);
  assert.throws(() => releaseNotes(source.replace('### Fixed', '### Added')), /Invalid or duplicate/);
  assert.throws(() => releaseNotes(source.replace('## [Unreleased]', '## Pending')), /Unreleased/);
  assert.throws(() => recordRelease(source, { ...options, notes: '' }), /without curated notes/);
});

test('reject invalid dates, versions and branches', () => {
  for (const date of ['15-10-2026', '2026-02-31', '2026-99-99']) {
    assert.throws(() => recordRelease(source, { ...options, date }), /valid YYYY-MM-DD/);
  }
  assert.throws(() => recordRelease(source, { ...options, version: 'bad]version' }), /Invalid release version/);
  assert.throws(() => recordRelease(source, { ...options, branch: 'stable' }), /Invalid release branch/);
});

test('editing and recording only changelog notes never starts another build', (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'depmanager-changelog-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  git('init', '-q');
  git('config', 'user.name', 'Changelog test');
  git('config', 'user.email', 'changelog-test@example.com');
  fs.writeFileSync(path.join(root, 'app.go'), 'package main\n');
  fs.writeFileSync(path.join(root, 'CHANGELOG.md'), source);
  git('add', '.');
  git('commit', '-qm', 'initial');
  const before = git('rev-parse', 'HEAD');
  fs.writeFileSync(path.join(root, 'CHANGELOG.md'), recordRelease(source, options));
  git('add', '.');
  git('commit', '-qm', 'record published notes');
  assert.deepEqual(findCodeChanges(root, before), []);
});

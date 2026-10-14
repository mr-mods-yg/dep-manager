const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { parseVersion, compareVersions } = require('./semver.cjs');

function nextReleaseVersion(target, channel, tags, publishedStableVersions = []) {
  const parsed = parseVersion(target);
  if (parsed.prerelease.length || parsed.build) throw new Error('The release target must be a plain MAJOR.MINOR.PATCH version');
  if (parsed.core.some(part => part > 65535n)) throw new Error('The release target exceeds Windows numeric version limits');
  if (!['nightly', 'stable'].includes(channel)) throw new Error('Invalid release channel');
  for (const stable of publishedStableVersions) {
    const version = stable.replace(/^v/, '');
    const published = parseVersion(version);
    if (published.prerelease.length) throw new Error('Published stable versions cannot have prerelease identifiers');
    if (compareVersions(target, version) <= 0) throw new Error('Choose a release target newer than published stable ' + version);
  }
  const versions = tags.map(tag => tag.replace(/^v/, ''));
  if (versions.includes(target)) throw new Error('The stable target already has a tag; choose a new release target');
  if (channel === 'stable') return target;
  // Nightly uses SemVer alpha identifiers; stable has no prerelease suffix.
  const prefix = target + '-alpha.';
  let counter = 0n;
  for (const version of versions) {
    if (!version.startsWith(prefix)) continue;
    const suffix = version.slice(prefix.length);
    if (/^[1-9]\d*$/.test(suffix) && BigInt(suffix) > counter) counter = BigInt(suffix);
  }
  return prefix + (counter + 1n);
}

function bumpVersion(root, options = {}) {
  const plan = JSON.parse(fs.readFileSync(path.join(root, 'updates/release-plan.json'), 'utf8'));
  const channel = options.channel ?? 'nightly';
  const tags = options.tags ?? execFileSync('git', ['tag', '--list'], { cwd: root, encoding: 'utf8' }).trim().split('\n').filter(Boolean);
  const version = nextReleaseVersion(plan.version, channel, tags, options.publishedStableVersions);
  const numericVersion = plan.version;
  const configPath = 'build/config.yml';
  const config = fs.readFileSync(path.join(root, configPath), 'utf8');
  const versionPattern = /^  version: "(\d+)\.(\d+)\.(\d+)" # The application version$/m;
  const match = config.match(versionPattern);
  if (!match) throw new Error('Could not find the application version in build/config.yml');

  // Windows fixed version fields and NSIS require a numeric version.
  const files = new Map([[configPath, config.replace(versionPattern, '  version: "' + numericVersion + '" # The application version')]]);

  function setJsonVersion(filePath, update) {
    const data = JSON.parse(fs.readFileSync(path.join(root, filePath), 'utf8'));
    update(data);
    files.set(filePath, JSON.stringify(data, null, 2) + '\n');
  }
  setJsonVersion('frontend/package.json', (data) => {
    data.version = version;
    data.releaseChannel = channel;
  });
  setJsonVersion('frontend/package-lock.json', (data) => {
    data.version = version;
    data.packages[''].version = version;
  });
  setJsonVersion('build/windows/info.json', (data) => {
    data.fixed.file_version = numericVersion;
    data.info['0000'].ProductVersion = version;
  });

  const nfpmPath = 'build/linux/nfpm/nfpm.yaml';
  const nfpm = fs.readFileSync(path.join(root, nfpmPath), 'utf8');
  const nfpmVersionPattern = /^version: "[^"]+"$/m;
  if (!nfpmVersionPattern.test(nfpm)) throw new Error('Could not find the package version in ' + nfpmPath);
  files.set(nfpmPath, nfpm.replace(nfpmVersionPattern, 'version: "' + version + '"'));

  const nsisPath = 'build/windows/nsis/wails_tools.nsh';
  const nsis = fs.readFileSync(path.join(root, nsisPath), 'utf8');
  const nsisVersionPattern = /!define INFO_PRODUCTVERSION "[^"]+"/;
  if (!nsisVersionPattern.test(nsis)) throw new Error('Could not find the installer version in ' + nsisPath);
  files.set(nsisPath, nsis.replace(nsisVersionPattern, '!define INFO_PRODUCTVERSION "' + numericVersion + '"'));

  // Validate every input before changing any version file.
  for (const [filePath, content] of files) fs.writeFileSync(path.join(root, filePath), content);
  return version;
}

if (require.main === module) {
  const args = process.argv.slice(2);
  if (args.length !== 1 || !['--nightly', '--stable'].includes(args[0])) {
    throw new Error('Usage: node updates/bump-version.cjs --nightly|--stable');
  }
  process.stdout.write(bumpVersion(path.resolve(__dirname, '..'), {
    channel: args[0].slice(2),
    publishedStableVersions: JSON.parse(process.env.PUBLISHED_STABLE_VERSIONS || '[]'),
  }) + '\n');
}

module.exports = { bumpVersion, nextReleaseVersion };

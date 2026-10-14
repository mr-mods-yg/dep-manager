const fs = require('node:fs');
const path = require('node:path');

function bumpVersion(root, nightlyNumber, options = {}) {
  if (nightlyNumber !== undefined && !/^[1-9]\d*$/.test(String(nightlyNumber))) {
    throw new Error('The nightly build number must be a positive integer');
  }
  const configPath = 'build/config.yml';
  const config = fs.readFileSync(path.join(root, configPath), 'utf8');
  const versionPattern = /^  version: "(\d+)\.(\d+)\.(\d+)" # The application version$/m;
  const match = config.match(versionPattern);
  if (!match) throw new Error('Could not find the application version in build/config.yml');

  const channel = options.channel ?? (nightlyNumber === undefined ? 'patch' : 'nightly');
  if (!['patch', 'nightly', 'beta', 'stable'].includes(channel)) throw new Error('Invalid release channel');
  const parse = (version) => {
    const parts = String(version).match(/^v?(\d+)\.(\d+)\.(\d+)(?:-[\w.-]+)?$/);
    if (!parts) throw new Error('Invalid release version: ' + version);
    return parts.slice(1, 4).map(Number);
  };
  const compare = (a, b) => a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
  const max = (...versions) => versions.reduce((a, b) => compare(a, b) >= 0 ? a : b);
  const current = match.slice(1, 4).map(Number);
  const stable = options.stableVersion ? parse(options.stableVersion) : [0, 0, 0];
  const beta = options.betaVersion ? parse(options.betaVersion) : [0, 0, 0];
  let next;
  if (channel === 'nightly') {
    const base = max(current, stable, beta);
    next = [base[0], base[1], base[2] + 1];
  } else if (channel === 'beta') {
    next = max(current, [stable[0], stable[1] + 1, 0]);
  } else if (channel === 'stable') {
    next = max(current, beta, [1, 0, 0]);
    if (options.stableVersion && compare(next, stable) <= 0) next = [stable[0], stable[1], stable[2] + 1];
  } else {
    next = [current[0], current[1], current[2] + 1];
  }
  // Windows fixed version fields and NSIS require a numeric version.
  const numericVersion = next.join('.');
  const version = numericVersion + (['nightly', 'beta'].includes(channel) ? '-' + channel + '.' + nightlyNumber : '');
  const files = new Map([[configPath, config.replace(versionPattern, '  version: "' + numericVersion + '" # The application version')]]);

  function setJsonVersion(filePath, update) {
    const data = JSON.parse(fs.readFileSync(path.join(root, filePath), 'utf8'));
    update(data);
    files.set(filePath, JSON.stringify(data, null, 2) + '\n');
  }
  setJsonVersion('frontend/package.json', (data) => {
    data.version = version;
    if (channel !== 'patch') data.releaseChannel = channel;
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
  if (args.length && (args.length !== 2 || !['--nightly', '--beta', '--stable'].includes(args[0]))) {
    throw new Error('Usage: node updates/bump-version.cjs [--nightly|--beta|--stable BUILD_NUMBER]');
  }
  process.stdout.write(bumpVersion(path.resolve(__dirname, '..'), args[1], {
    channel: args[0]?.slice(2),
    stableVersion: process.env.STABLE_VERSION || undefined,
    betaVersion: process.env.BETA_VERSION || undefined,
  }) + '\n');
}

module.exports = { bumpVersion };

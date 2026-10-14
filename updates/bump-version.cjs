const fs = require('node:fs');
const path = require('node:path');

function bumpVersion(root, nightlyNumber) {
  if (nightlyNumber !== undefined && !/^[1-9]\d*$/.test(String(nightlyNumber))) {
    throw new Error('The nightly build number must be a positive integer');
  }
  const configPath = 'build/config.yml';
  const config = fs.readFileSync(path.join(root, configPath), 'utf8');
  const versionPattern = /^  version: "(\d+)\.(\d+)\.(\d+)" # The application version$/m;
  const match = config.match(versionPattern);
  if (!match) throw new Error('Could not find the application version in build/config.yml');

  // Windows fixed version fields and NSIS require a numeric version.
  const numericVersion = match[1] + '.' + match[2] + '.' + (Number(match[3]) + 1);
  const version = numericVersion + (nightlyNumber === undefined ? '' : '-nightly.' + nightlyNumber);
  const files = new Map([[configPath, config.replace(versionPattern, '  version: "' + numericVersion + '" # The application version')]]);

  function setJsonVersion(filePath, update) {
    const data = JSON.parse(fs.readFileSync(path.join(root, filePath), 'utf8'));
    update(data);
    files.set(filePath, JSON.stringify(data, null, 2) + '\n');
  }
  setJsonVersion('frontend/package.json', (data) => { data.version = version; });
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
  if (args.length && (args.length !== 2 || args[0] !== '--nightly')) {
    throw new Error('Usage: node updates/bump-version.cjs [--nightly BUILD_NUMBER]');
  }
  process.stdout.write(bumpVersion(path.resolve(__dirname, '..'), args[1]) + '\n');
}

module.exports = { bumpVersion };

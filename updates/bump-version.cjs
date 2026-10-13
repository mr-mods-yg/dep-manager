const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(__dirname, '..');
const configPath = path.join(root, 'build/config.yml');
const config = fs.readFileSync(configPath, 'utf8');
const versionPattern = /^  version: "(\d+)\.(\d+)\.(\d+)" # The application version$/m;
const match = config.match(versionPattern);

if (!match) {
  throw new Error('Could not find the application version in build/config.yml');
}

const nextVersion = `${match[1]}.${match[2]}.${Number(match[3]) + 1}`;
const updatedConfig = config.replace(versionPattern, `  version: "${nextVersion}" # The application version`);
fs.writeFileSync(configPath, updatedConfig);

function setJsonVersion(filePath, update) {
  const absolutePath = path.join(root, filePath);
  const data = JSON.parse(fs.readFileSync(absolutePath, 'utf8'));
  update(data);
  fs.writeFileSync(absolutePath, `${JSON.stringify(data, null, 2)}\n`);
}

setJsonVersion('frontend/package.json', (data) => {
  data.version = nextVersion;
});

setJsonVersion('frontend/package-lock.json', (data) => {
  data.version = nextVersion;
  data.packages[''].version = nextVersion;
});

setJsonVersion('build/windows/info.json', (data) => {
  data.fixed.file_version = nextVersion;
  data.info['0000'].ProductVersion = nextVersion;
});

const nfpmPath = path.join(root, 'build/linux/nfpm/nfpm.yaml');
const nfpm = fs.readFileSync(nfpmPath, 'utf8');
const nfpmVersionPattern = /^version: "\d+\.\d+\.\d+"$/m;
if (!nfpmVersionPattern.test(nfpm)) {
  throw new Error('Could not find the package version in build/linux/nfpm/nfpm.yaml');
}
fs.writeFileSync(nfpmPath, nfpm.replace(nfpmVersionPattern, `version: "${nextVersion}"`));

process.stdout.write(`${nextVersion}\n`);

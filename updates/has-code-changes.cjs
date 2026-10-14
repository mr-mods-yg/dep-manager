const { execFileSync } = require('node:child_process');
const path = require('node:path');

function isBuildInput(file) {
  if (file.endsWith('.md') || file === 'updates/release-plan.json') return false;
  return /^[^/]+\.go$/.test(file)
    || ['go.mod', 'go.sum', 'Taskfile.yml'].includes(file)
    || file.startsWith('packages/')
    || file.startsWith('updates/')
    || file.startsWith('.github/workflows/')
    || file === 'build/Taskfile.yml'
    || file === 'build/config.yml'
    || file === 'build/appicon.png'
    || file.startsWith('build/windows/')
    || file.startsWith('build/linux/')
    || file.startsWith('frontend/src/')
    || file.startsWith('frontend/public/')
    || file.startsWith('frontend/bindings/')
    || /^frontend\/(package(?:-lock)?\.json|index\.html|tsconfig.*\.json|vite\.config\..+|\.npmrc)$/.test(file);
}

function normalise(file, content) {
  if (content === null) return null;
  if (['frontend/package.json', 'frontend/package-lock.json', 'build/windows/info.json'].includes(file)) {
    const data = JSON.parse(content);
    if (file === 'build/windows/info.json') {
      delete data.fixed.file_version;
      delete data.info['0000'].ProductVersion;
    } else {
      delete data.version;
      delete data.releaseChannel;
      if (file.endsWith('package-lock.json')) delete data.packages[''].version;
    }
    return JSON.stringify(data);
  }
  if (file === 'build/config.yml') return content.replace(/^  version: "[^"]+" # The application version$/m, '');
  if (file === 'build/linux/nfpm/nfpm.yaml') return content.replace(/^version: "[^"]+"$/m, '');
  if (file === 'build/windows/nsis/wails_tools.nsh') return content.replace(/!define INFO_PRODUCTVERSION "[^"]+"/, '');
  return content;
}

function findCodeChanges(root, base, head = 'HEAD') {
  if (!base) return ['initial build'];
  const git = (args) => execFileSync('git', args, { cwd: root, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe'] });
  // Fail on an invalid baseline instead of silently publishing another build.
  git(['rev-parse', '--verify', base + '^{commit}']);
  git(['rev-parse', '--verify', head + '^{commit}']);
  const files = git(['diff', '--name-only', '--no-renames', '-z', base, head, '--']).split('\0').filter(isBuildInput);
  const read = (ref, file) => {
    if (!git(['ls-tree', '--name-only', ref, '--', file]).trim()) return null;
    return git(['show', ref + ':' + file]);
  };
  return files.filter((file) => normalise(file, read(base, file)) !== normalise(file, read(head, file)));
}

if (require.main === module) {
  const [, , base, head] = process.argv;
  const changed = findCodeChanges(path.resolve(__dirname, '..'), base, head);
  if (changed.length) process.stderr.write('Changed build inputs: ' + changed.join(', ') + '\n');
  process.stdout.write(String(changed.length > 0) + '\n');
}

module.exports = { findCodeChanges };

// SemVer 2.0.0: numeric identifiers have no leading zeroes; build metadata
// never affects precedence. BigInt preserves ordering for large identifiers.
const pattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/;

function parseVersion(version) {
  const match = typeof version === 'string' && version.match(pattern);
  if (!match) throw new Error('Invalid SemVer version: ' + version);
  return { core: match.slice(1, 4).map(BigInt), prerelease: match[4]?.split('.') ?? [], build: match[5] };
}

function compareVersions(left, right) {
  const a = parseVersion(left);
  const b = parseVersion(right);
  const compare = (x, y) => x < y ? -1 : x > y ? 1 : 0;
  for (let index = 0; index < 3; index++) {
    const result = compare(a.core[index], b.core[index]);
    if (result) return result;
  }
  if (!a.prerelease.length || !b.prerelease.length) return compare(!a.prerelease.length, !b.prerelease.length);
  for (let index = 0; index < Math.max(a.prerelease.length, b.prerelease.length); index++) {
    const x = a.prerelease[index];
    const y = b.prerelease[index];
    if (x === undefined || y === undefined) return compare(x !== undefined, y !== undefined);
    const xNumeric = /^\d+$/.test(x);
    const yNumeric = /^\d+$/.test(y);
    const result = xNumeric && yNumeric ? compare(BigInt(x), BigInt(y))
      : xNumeric !== yNumeric ? compare(!xNumeric, !yNumeric) : compare(x, y);
    if (result) return result;
  }
  return 0;
}

module.exports = { parseVersion, compareVersions };

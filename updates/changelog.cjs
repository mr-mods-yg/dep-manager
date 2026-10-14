const fs = require('node:fs');
const path = require('node:path');

const categories = ['Added', 'Changed', 'Deprecated', 'Removed', 'Fixed', 'Security'];
const repository = 'https://github.com/mr-mods-yg/dep-manager';

function document(source) {
  const text = source.replace(/\r\n/g, '\n');
  const referenceStart = text.search(/^\[[^\]]+\]:\s+\S+/m);
  const bodyEnd = referenceStart < 0 ? text.length : referenceStart;
  const headings = [...text.slice(0, bodyEnd).matchAll(/^## \[([^\]\n]+)\](?: - (\d{4}-\d{2}-\d{2}))?(?: \[YANKED\])?[ \t]*$/gm)];
  const sections = headings.map((heading, index) => ({
    version: heading[1],
    start: heading.index,
    contentStart: heading.index + heading[0].length,
    end: headings[index + 1]?.index ?? bodyEnd,
  }));
  if (sections.filter(s => s.version === 'Unreleased').length !== 1 || sections[0]?.version !== 'Unreleased') {
    throw new Error('CHANGELOG.md must start its release history with one ## [Unreleased] section');
  }
  return { text, sections, references: text.slice(bodyEnd) };
}

function changes(content) {
  const text = content.trim();
  if (!text) return new Map();
  const headings = [...text.matchAll(/^### (.+)[ \t]*$/gm)];
  if (!headings.length || text.slice(0, headings[0].index).trim()) {
    throw new Error('Group changelog notes under Added, Changed, Deprecated, Removed, Fixed, or Security');
  }
  const groups = new Map();
  for (let index = 0; index < headings.length; index++) {
    const category = headings[index][1].trim();
    if (!categories.includes(category) || groups.has(category)) throw new Error('Invalid or duplicate changelog category: ' + category);
    const body = text.slice(headings[index].index + headings[index][0].length, headings[index + 1]?.index ?? text.length).trim();
    if (!body) continue;
    if (!/^[-*] /.test(body)) throw new Error('Write changelog entries as bullet points under ' + category);
    groups.set(category, body.split(/(?=^[-*] )/m).map(entry => entry.trim()).filter(Boolean));
  }
  return groups;
}

function format(groups) {
  return categories.filter(category => groups.get(category)?.length)
    .map(category => '### ' + category + '\n\n' + groups.get(category).join('\n'))
    .join('\n\n');
}

function releaseNotes(source, version = 'Unreleased') {
  const doc = document(source);
  const section = doc.sections.find(s => s.version === version);
  if (!section) throw new Error('No changelog entry exists for ' + version);
  const notes = format(changes(doc.text.slice(section.contentStart, section.end)));
  if (!notes) throw new Error('Add curated changes under ## [Unreleased] before releasing code changes');
  // Keep reference-style links in user-written notes usable on GitHub.
  return notes + '\n' + (doc.references.trim() ? '\n' + doc.references.trim() + '\n' : '');
}

function recordRelease(source, { version, date, branch, notes }) {
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) throw new Error('Invalid release version');
  const parsedDate = new Date(date + 'T00:00:00Z');
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || Number.isNaN(parsedDate.getTime()) || parsedDate.toISOString().slice(0, 10) !== date) {
    throw new Error('Use a valid YYYY-MM-DD release date');
  }
  if (!['nightly', 'beta', 'main'].includes(branch)) throw new Error('Invalid release branch');
  const doc = document(source);
  // Rerunning a publication must preserve an already recorded, curated entry.
  if (doc.sections.some(s => s.version === version)) return doc.text;
  const noteBody = notes.replace(/^\[[^\]]+\]:\s+\S+.*$/gm, '').trim();
  const released = changes(noteBody);
  if (!format(released)) throw new Error('Cannot record a release without curated notes');
  const unreleased = doc.sections[0];
  const remaining = changes(doc.text.slice(unreleased.contentStart, unreleased.end));
  for (const category of categories) {
    const published = new Set(released.get(category) ?? []);
    if (remaining.has(category)) remaining.set(category, remaining.get(category).filter(entry => !published.has(entry)));
  }
  const pending = format(remaining);
  const release = '## [' + version + '] - ' + date + '\n\n' + format(released);
  const previous = doc.sections[1]?.version;
  const compare = previous ? repository + '/compare/v' + previous + '...v' + version : repository + '/releases/tag/v' + version;
  const references = doc.references.split('\n').filter(line => line.trim() && !/^\[unreleased\]:/i.test(line));
  const keys = new Set(references.map(line => line.match(/^\[([^\]]+)\]:/)?.[1].toLowerCase()).filter(Boolean));
  for (const line of notes.split('\n')) {
    const key = line.match(/^\[([^\]]+)\]:\s+\S+/)?.[1].toLowerCase();
    if (key && key !== 'unreleased' && key !== version.toLowerCase() && !keys.has(key)) {
      references.push(line);
      keys.add(key);
    }
  }
  const links = '[Unreleased]: ' + repository + '/compare/v' + version + '...' + branch + '\n'
    + '[' + version + ']: ' + compare + '\n' + references.join('\n');
  return [
    doc.text.slice(0, unreleased.contentStart).trimEnd(),
    pending, release,
    doc.text.slice(unreleased.end, doc.text.length - doc.references.length).trim(),
    links.trim(),
  ].filter(Boolean).join('\n\n') + '\n';
}

if (require.main === module) {
  const [command, ...args] = process.argv.slice(2);
  const file = path.resolve(__dirname, '..', 'CHANGELOG.md');
  const source = fs.readFileSync(file, 'utf8');
  if (command === 'notes' && args.length <= 1) {
    process.stdout.write(releaseNotes(source, args[0]));
  } else if (command === 'record' && args.length === 4) {
    const [version, date, branch, notesFile] = args;
    const updated = recordRelease(source, { version, date, branch, notes: fs.readFileSync(notesFile, 'utf8') });
    if (updated !== source) fs.writeFileSync(file, updated);
  } else {
    throw new Error('Usage: node updates/changelog.cjs notes [VERSION] | record VERSION DATE BRANCH NOTES_FILE');
  }
}

module.exports = { releaseNotes, recordRelease };

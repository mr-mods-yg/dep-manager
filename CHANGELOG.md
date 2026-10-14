# Changelog

Notable changes in DepManager are recorded here.

This file follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
This history starts with the reset prerelease series.

## [Unreleased]

### Added

- A Beta option alongside Stable and Nightly for automatic and manual application updates.
- Repository instructions in AGENTS.md for branches, semantic versioning, updates, and changelog maintenance.

### Changed

- Follow SemVer with a shared release target: alpha versions for Nightly, beta versions for Beta, and plain versions for Stable.
- Number prereleases per target and channel instead of using GitHub Actions run numbers.
- Choose compatible update releases by semantic version precedence across every release page.

## [0.0.6-nightly.8] - 2026-10-14

### Added

- A repository changelog with categorized, dated release history.
- Automatic archiving of published versions with UTC dates and comparison links.

### Changed

- Publish human-written changelog entries as GitHub release notes instead of generated commit summaries.
- Use main for stable release promotions, alongside beta and nightly.

## [0.0.5-nightly.5] - 2026-10-14

### Added

- A saved Stable/Nightly update channel selector for automatic and manual updates.
- Verified Windows and Debian/Ubuntu application updates using release checksums.
- Live progress for package updates, repairs, and removals.

### Changed

- Start the new nightly version series at 0.0.5, with beta and stable promotions managed separately.
- Publish builds only when application code, dependencies, assets, or build scripts change.

### Fixed

- Reopen the application visibly after Windows updates.
- Keep the application open when the Windows update helper cannot start.
- Preserve installation and restart errors for the next application launch.
- Clear the previous update offer when switching update channels.

[Unreleased]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.6-nightly.8...nightly
[0.0.6-nightly.8]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.5-nightly.5...v0.0.6-nightly.8
[0.0.5-nightly.5]: https://github.com/mr-mods-yg/dep-manager/releases/tag/v0.0.5-nightly.5

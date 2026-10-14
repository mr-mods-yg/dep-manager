# Archived changelog

These releases were withdrawn on 2026-10-14 before restarting the development series at 0.0.1.
Their original notes and Git tags are preserved here for history.

## [1.0.0-alpha.1] - 2026-10-14 [YANKED]

### Added

- A Beta option alongside Stable and Nightly for automatic and manual application updates.
- Repository instructions in AGENTS.md for branches, semantic versioning, updates, and changelog maintenance.

### Changed

- Follow SemVer with a shared release target: alpha versions for Nightly, beta versions for Beta, and plain versions for Stable.
- Number prereleases per target and channel instead of using GitHub Actions run numbers.
- Choose compatible update releases by semantic version precedence across every release page.

## [0.0.6-nightly.8] - 2026-10-14 [YANKED]

### Added

- A repository changelog with categorized, dated release history.
- Automatic archiving of published versions with UTC dates and comparison links.

### Changed

- Publish human-written changelog entries as GitHub release notes instead of generated commit summaries.
- Use main for stable release promotions, alongside beta and nightly.

## [0.0.5-nightly.5] - 2026-10-14 [YANKED]

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

[1.0.0-alpha.1]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.6-nightly.8...v1.0.0-alpha.1
[0.0.6-nightly.8]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.5-nightly.5...v0.0.6-nightly.8
[0.0.5-nightly.5]: https://github.com/mr-mods-yg/dep-manager/tree/v0.0.5-nightly.5

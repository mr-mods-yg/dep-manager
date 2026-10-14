# Changelog

Notable changes in DepManager are recorded here.

This file follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
The fresh development series starts at 0.0.1.
Earlier withdrawn releases are documented in [CHANGELOG-ARCHIVE.md](CHANGELOG-ARCHIVE.md).

## [Unreleased]

## [0.0.3-alpha.1] - 2026-10-14

### Changed

- Organize package manager services, command helpers, progress handling, and tests under `packages/`.

## [0.0.2] - 2026-10-14

### Added

- Identify pnpm's own global package and ask before updating it with `pnpm self-update`.

### Changed

- Fetch manager versions and global package lists concurrently, and check up to eight latest package versions at a time.

### Fixed

- Start the Windows update helper without a console window so PowerShell runs the installer script instead of exiting before it is ready.

## [0.0.2-alpha.2] - 2026-10-14

### Added

- Identify pnpm's own global package and ask before updating it with `pnpm self-update`.

### Changed

- Fetch manager versions and global package lists concurrently, and check up to eight latest package versions at a time.

## [0.0.2-alpha.1] - 2026-10-14

### Fixed

- Start the Windows update helper without a console window so PowerShell runs the installer script instead of exiting before it is ready.

## [0.0.1] - 2026-10-14

### Added

- Manage global JavaScript packages with updates, repairs, removals, and live progress.
- Saved Stable and Nightly channels for automatic and manual application updates.
- Verified Windows and Debian/Ubuntu installers using SHA256SUMS.
- Curated changelog history and repository instructions in AGENTS.md.

### Changed

- Promote the tested 0.0.1 Nightly series directly to Stable on main.
- Publish builds only when application code, dependencies, assets, or build scripts change.
- Offer the highest eligible update by SemVer precedence across release pages.

### Removed

- The Beta branch, release channel, and update selector option.

### Fixed

- Keep the app open if the Windows update helper cannot start and reopen it visibly after installation.
- Preserve installation and restart errors for the next launch and clear stale update offers on channel changes.

## [0.0.1-alpha.2] - 2026-10-14

### Removed

- The Beta branch, release channel, and update selector option. Releases now progress directly from Nightly to Stable on main.

## [0.0.1-alpha.1] - 2026-10-14

### Added

- Manage global JavaScript packages with package updates, repairs, removals, and live progress.
- Saved Stable, Beta, and Nightly channels for automatic and manual application updates.
- Windows and Debian/Ubuntu installers with SHA256SUMS verification.
- Human-written changelog notes and repository instructions in AGENTS.md.

### Changed

- Restart the development series at 0.0.1 with alpha versions for Nightly, beta versions for Beta, and plain versions for Stable.
- Keep the base version fixed while iterating on a release; advance independent alpha and beta counters only when code changes.
- Offer the highest eligible update by SemVer precedence, including paginated releases.

### Fixed

- Keep the application open if the Windows update helper cannot start and reopen it visibly after installation.
- Preserve installation and restart errors for the next launch and clear stale update offers on channel changes.

[Unreleased]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.3-alpha.1...nightly
[0.0.3-alpha.1]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.2...v0.0.3-alpha.1
[0.0.2]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.2-alpha.2...v0.0.2
[0.0.2-alpha.2]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.2-alpha.1...v0.0.2-alpha.2
[0.0.2-alpha.1]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.1...v0.0.2-alpha.1
[0.0.1]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.1-alpha.2...v0.0.1
[0.0.1-alpha.2]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.1-alpha.1...v0.0.1-alpha.2
[0.0.1-alpha.1]: https://github.com/mr-mods-yg/dep-manager/releases/tag/v0.0.1-alpha.1

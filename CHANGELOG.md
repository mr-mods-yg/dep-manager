# Changelog

Notable changes in DepManager are recorded here.

This file follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
The fresh development series starts at 0.0.1.
Earlier withdrawn releases are documented in [CHANGELOG-ARCHIVE.md](CHANGELOG-ARCHIVE.md).

## [Unreleased]

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

[Unreleased]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.1-alpha.2...nightly
[0.0.1-alpha.2]: https://github.com/mr-mods-yg/dep-manager/compare/v0.0.1-alpha.1...v0.0.1-alpha.2
[0.0.1-alpha.1]: https://github.com/mr-mods-yg/dep-manager/releases/tag/v0.0.1-alpha.1

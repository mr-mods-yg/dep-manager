# DepManager repository instructions

## Branches and release channels

- `main` is the stable and default branch. Do not create a `stable` branch.
- `beta` is for beta releases; `nightly` is for development releases.
- Nightly pushes can publish automatically. Beta and stable releases require an intentional workflow dispatch on their respective branches.
- Keep the release workflow and shared updater/versioning code consistent across all three branches. Promote tested changes from nightly to beta, then main.

## Semantic Versioning

Follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

- `updates/release-plan.json` declares the next target as plain `MAJOR.MINOR.PATCH`. The first target is `1.0.0`.
- All stages of one release use the same target: Nightly `1.0.0-alpha.1`, Beta `1.0.0-beta.1`, Stable `1.0.0`.
- The Nightly channel uses `alpha` in version identifiers because SemVer sorts it before `beta`. Retain the name Nightly in the app, branch, and GitHub release titles.
- Increment PATCH for compatible bug fixes, MINOR for compatible features or deprecations, and MAJOR for incompatible public behavior/API changes. Reset lower components when increasing a higher component. Choose the target explicitly; do not infer semantic impact from the branch name or workflow run count.
- The compatibility contract covers documented package actions/manager support, saved settings, and the update protocol (version/channel metadata, platform asset names, and SHA256SUMS). Version `1.0.0` establishes this contract. Major-zero versions indicate initial development.
- Use `node updates/bump-version.cjs --nightly`, `--beta`, or `--stable` to update all packaging metadata together. Keep Windows fixed/NSIS versions numeric; embed the full SemVer version in the app.
- Prerelease counters are independent for each target/channel and derived from reserved Git tags. They start at 1 for a new target. Failed tagged builds reserve their counter; unchanged or documentation-only runs consume no version number.
- Only publish when application code, dependencies, assets, or build scripts differ from the last published release of that channel. Version metadata, release-plan changes, and documentation alone do not trigger publication.
- Never reuse a tag or replace a published version's binaries. Choose a new version for changed contents. Nightly and beta are GitHub prereleases; stable is a regular release.

## Application updates

- Preserve the saved Stable/Beta/Nightly selector for both automatic and manual updates.
- Stable accepts stable releases. Beta accepts beta and stable releases. Nightly accepts alpha, beta, stable, and legacy `-nightly.N` releases.
- Select the highest eligible version by SemVer precedence, including paginated releases. Ignore drafts, malformed versions, and mismatched prerelease flags. Build metadata does not affect precedence. Do not automatically downgrade when switching channels.
- Clear a pending update offer whenever its channel changes or a check fails. Require a valid SHA256SUMS entry before installation.
- Keep Windows restart visible, wait for helper readiness before quitting, and preserve installation/restart failures for the next launch.

## Changelog and verification

- Maintain `CHANGELOG.md` using [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/): one Unreleased section, newest releases first, UTC `YYYY-MM-DD` dates, comparison links, and relevant Added/Changed/Deprecated/Removed/Fixed/Security categories.
- Add human-written Unreleased notes with code changes. GitHub release notes come from this snapshot. Archive it only after successful publication, preserving notes added during the build. Failed builds leave notes pending.
- Check release automation with `node --test updates/*.test.cjs` and actionlint. Check updater changes with `go test ./updates`, and frontend changes with `npm run build` in `frontend`.
- In a headless environment without GTK, use `CGO_ENABLED=0 go test -tags 'server,production' ./...`. PowerShell handoff tests need `pwsh`; the Windows release job exercises them on Windows.

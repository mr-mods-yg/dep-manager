# depdesk

depdesk is a Wails desktop app for viewing global JavaScript packages installed by npm, pnpm, and Bun. It detects the package manager CLIs available on the current machine, shows each tool's version and global packages, and can update a package or remove it.

## Run locally

Install Go, Node.js, and the Wails v3 CLI, then run:

```sh
wails3 dev -config ./build/config.yml -port 9245
```

The frontend is in `frontend/`; package manager discovery and actions are implemented in `packageservice.go`.

## Package manager support

- npm and pnpm global packages are read from their JSON list commands.
- Bun packages are read from Bun's configured global install directory, including the default location.
- Depdesk checks each installed package against its registry's `latest` tag and marks matching versions as already latest. If a registry check is unavailable, the update action remains available.
- Update applies the package manager's global update command. Remove asks for confirmation before uninstalling.

Only managers found on the current system are enabled. A **Scan system** refreshes the detected managers and package list.

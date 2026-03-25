# Changelog

All notable changes to this project will be documented in this file.

## [0.8.0] - 2026-03-25

### Added
- Interactive create wizard using Bubble Tea (TUI) for guided workflow when terminal supports it.
- New `pubget` command to run `pub get` in all package repos for a managed workspace in parallel, with root-final ordering.
- `--force` option for `pubget` command to clean cache and remove pubspec.lock before pub get.
- Tabular terminal output improvements with better formatting and interactive selection features.
- New UI components using Bubble Tea and Lip Gloss for enhanced terminal experience.
- Integration tests for CLI contract, docs contract, and release verification.

### Changed
- Improved workspace lifecycle safety by hiding package rows in list command.
- Enhanced `complete` command to cascade across associated packages and remove managed root containers.
- Auto-ignore `pubspec_overrides.yaml` in .gitignore when creating workspaces.
- Generate per-package branches based on workspace name for better organization.
- Update create dry-run plan to support interactive and non-interactive modes consistently.
- Safer completion flow that handles associated packages and validates container paths.

### Fixed
- Container path validation for worktree completion to prevent unsafe removals.
- Race conditions in parallel package operations during pubget execution.
- Gitignore handling to prevent duplicate entries for pubspec_overrides.yaml.

## [0.7.0] - 2026-03-25

### Added
- New Go CLI entrypoint at `cmd/flutree`.
- Layered Go architecture under `internal/`:
  - `internal/app`
  - `internal/domain`
  - `internal/infra`
  - `internal/runtime`
  - `internal/ui`
- Go test suite bootstrap for core services.
- Release pipeline alignment for Go binary packaging.

### Changed
- Migrated core command flows (`create`, `list`, `complete`) from Python to Go.
- Updated release packaging script to build with `go build`.
- Updated documentation for Go-first development and testing.

### Removed
- Legacy runtime and test stack replaced by Go implementation.

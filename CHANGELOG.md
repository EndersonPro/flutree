# Changelog

All notable changes to this project will be documented in this file.

## [0.10.0](https://github.com/EndersonPro/flutree/compare/v0.9.2...v0.10.0) (2026-03-26)


### Features

* add explicit root-only create and global list scopes ([6a50f67](https://github.com/EndersonPro/flutree/commit/6a50f679b8400d1501ec376fca5dcbe5092042da))
* flutree go-only baseline v0.7.0 ([6109569](https://github.com/EndersonPro/flutree/commit/6109569e7b17e323cdd0a28376e928672400fe76))
* harden worktree lifecycle and add CLI version/update contracts ([c8dce36](https://github.com/EndersonPro/flutree/commit/c8dce36b832fc15c45eada4c33dfe7433e40336c))
* refine create flow and document recent CLI updates ([5e13925](https://github.com/EndersonPro/flutree/commit/5e13925852ab1ea96e9d6b000e04e9d83934323f))
* release v0.8.0 with interactive workflows and pubget ([0eb1b31](https://github.com/EndersonPro/flutree/commit/0eb1b3139abb60f8f31f80ff1694fb426a6669ee))

## [0.9.0] - 2026-03-25

### Added
- Stable CLI version surface with both `flutree --version` and `flutree version`.
- Brew-only auto-update commands: `flutree update --check`, `flutree update --apply`, and `flutree update` (default apply path), including machine-parseable output.
- Update service and Homebrew gateway with test coverage for check/apply and unavailable-brew failure paths.

### Changed
- `create` now requires explicit confirmation before reusing an existing branch; non-interactive mode requires `--reuse-existing-branch`.
- New-branch creation now syncs the configured base branch before creating worktrees and blocks creation if sync fails.
- Package branch derivation now preserves the exact explicit/default root branch token deterministically.
- Documentation now includes the version/update command contract, update mode behavior, and brew-only scope.

### Fixed
- `complete` now treats missing worktree paths as stale registry drift, performs cleanup, and returns success.
- Integration fixtures now provision a local origin in test repositories so base-branch sync checks run consistently.

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

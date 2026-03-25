# Changelog

All notable changes to this project will be documented in this file.

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

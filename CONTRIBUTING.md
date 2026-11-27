# Contributing to flutree

Thank you for your interest in contributing to `flutree`! All contributions are welcome. Please read the following guidelines to get started.

## Table of Contents
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Testing](#testing)
- [Pull Requests](#pull-requests)
- [Style Guides](#style-guides)
- [Community](#community)

## Development Setup

### Prerequisites
- Go `>=1.22`
- Git available in `PATH`
- Access to a Flutter project for testing (optional)

### Getting Started
1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/flutree.git`
3. Navigate to the project directory: `cd flutree`
4. Install dependencies: `go mod download`
5. Build the project: `go build -o flutree ./cmd/flutree`
6. Enable the versioned Git hooks: `git config core.hooksPath .githooks`

## Project Structure

```
cmd/flutree/          # CLI entry point
internal/
  app/               # Application services and business logic
  domain/            # Domain models and interfaces
  infra/             # Infrastructure implementations (git, registry, etc.)
  runtime/           # Runtime utilities
  ui/                # User interface components (using Bubble Tea)
docs/                # Documentation
integration/         # End-to-end CLI tests (build and run the real binary)
scripts/             # Build and release scripts
```

## Testing

### Running Tests
```bash
# End-to-end CLI suite (the pre-push gate)
./scripts/e2e.sh

# Everything, including the remaining unit tests
go test ./...
```

### Pre-push hook
`.githooks/pre-push` runs `scripts/e2e.sh` on every `git push` and aborts the push when it fails.
Enable it once per clone with `git config core.hooksPath .githooks`. Skip it only in an emergency with `git push --no-verify`.

### Writing Tests
- **End-to-end first.** New behavior is covered by tests in `integration/` that build the `flutree` binary and run it the way a user does, asserting stdout, stderr, exit code and the files or Git state it leaves behind.
- Write a unit test only when an end-to-end test cannot reach the behavior (for example, a race, a rare error branch, or a pure parsing helper).
- Use the helpers in `integration/e2e_helpers_test.go`: `runE2E` (separate stdout/stderr), `createWorkspace` (real `flutree create`), and `newFakeToolchain` (stub `flutter`/`dart` that record each call).
- Keep every test hermetic: point `HOME` at `t.TempDir()` and never touch real repositories or user config.

## Pull Requests

### Before Submitting
- Ensure all tests pass (`./scripts/e2e.sh` runs automatically on push)
- Update documentation as needed
- Add tests for new features
- Follow the style guides below

### PR Process
1. Create a feature branch: `git checkout -b feature/amazing-feature`
2. Make your changes
3. Add tests if applicable
4. Run the full test suite
5. Update documentation
6. Commit your changes using conventional commits
7. Push to your fork
8. Open a pull request to the `main` branch

### PR Description Template
When creating your pull request, please include:
- A clear description of the changes
- The problem being solved (if applicable)
- How you tested the changes
- Any breaking changes (if applicable)

## Style Guides

### Go Style
- Follow [Effective Go](https://golang.org/doc/effective_go.html) guidelines
- Use `gofmt` to format your code
- Write clear, idiomatic Go code
- Document exported functions, types, and packages

### Git Style
- Use [Conventional Commits](https://www.conventionalcommits.org/)
- Keep commits focused and atomic
- Write clear, imperative commit messages

#### Commit Message Format
```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

Common types:
- `feat`: A new feature
- `fix`: A bug fix
- `docs`: Documentation only changes
- `style`: Changes that do not affect the meaning of the code
- `refactor`: A code change that neither fixes a bug nor adds a feature
- `test`: Adding missing tests or correcting existing tests
- `chore`: Other changes that don't modify src or test files

### Documentation Style
- Use Markdown for documentation
- Keep examples up-to-date
- Explain the "why" not just the "what"

## Community

### Need Help?
- Open an issue for bugs or feature requests
- For questions, create an issue with the "question" label
- Be respectful and constructive in all interactions

### Reporting Issues
When reporting issues, please include:
- Steps to reproduce the problem
- Expected behavior
- Actual behavior
- Environment details (OS, Go version, etc.)
- Any relevant screenshots or logs

## Code of Conduct

This project adheres to the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## Questions?

If you have any questions about contributing, feel free to open an issue with the "question" label.
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
integration/         # Integration tests
scripts/             # Build and release scripts
```

## Testing

### Running Tests
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test ./... -cover

# Run tests with verbose output
go test -v ./...
```

### Writing Tests
- Place unit tests alongside the code they test (e.g., `create_service_test.go` for `create_service.go`)
- Use table-driven tests where appropriate
- Maintain high test coverage, especially for critical business logic
- Write integration tests for complex workflows in the `integration/` directory

## Pull Requests

### Before Submitting
- Ensure all tests pass
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
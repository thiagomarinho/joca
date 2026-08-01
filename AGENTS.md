# Joca — Agent Instructions

These instructions apply to the entire repository.

## Build & Run

```bash
go build ./...
go run .
go test ./internal/...  # unit tests only
go test ./e2e/...       # e2e tests only (builds binary)
go test ./...           # everything
```

Versioned build:

```bash
go build -ldflags "-X github.com/thiagomarinho/joca/version.Version=0.1.0 -X github.com/thiagomarinho/joca/version.Commit=$(git rev-parse --short HEAD)" -o joca .
```

## Feature Workflow and Commits

Prefer test-driven development: write a failing test first, then implement the
smallest change that makes it pass.

At the end of every feature implementation:

1. Run `go test ./...` and confirm everything passes.
2. Provide clear instructions for manually testing the feature.
3. Wait for the user's confirmation before creating a commit.

Do not commit changes unless the user has explicitly confirmed the feature and
asked you to proceed with the commit.

## Code Quality

```bash
golangci-lint run ./...
govulncheck ./...
```

Run the relevant quality checks for the scope of the change. Report any checks
that could not be run and why.

## Testing Conventions

- Map `foo.go` to `foo_test.go` in the same package directory.
- Use `package foo_test` (an external test package) for black-box tests.
- Put end-to-end tests in `e2e/`; they must exercise the compiled binary.

## Package Layout

```text
cmd/                    # Cobra command definitions, one file per command
e2e/                    # End-to-end tests (builds the real binary; no mocks)
e2e/testdata/           # Fixture directories for end-to-end tests
internal/config/        # Config struct and ~/.joca path resolution
version/                # Version string, set via -ldflags at build time
```

## Key Conventions

- Use `RunE`, not `Run`, on Cobra commands so errors produce a non-zero exit.
- The package-level `cfg config.Config` variable in `cmd/root.go` is shared by
  all files in `cmd/`.
- Pass `config.Config` by value, not by pointer; it is small and set once.
- Wrap errors with context: `fmt.Errorf("doing X: %w", err)`.
- Group imports as standard library, external, and local, with a blank line
  between groups. This is enforced by `goimports`.
- Keep `RootCmd` exported so external test packages such as `cmd_test` can
  inspect registered subcommands.

# Joca — Claude Code Instructions

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

## Commits

At the end of every feature implementation:
1. Run `go test ./...` to confirm everything passes
2. Print instructions about how to test the feature manually
3. Wait for confirmation, then commit

## Code Quality

```bash
golangci-lint run ./...
govulncheck ./...
```

## Testing Convention

**Prefer TDD**: write the test first, then implement.

- `foo.go` → `foo_test.go` in the same package
- Use `package foo_test` (external test package) for black-box tests
- E2E tests live in `e2e/` and test the compiled binary

## Package Layout

```
cmd/                    # cobra command definitions, one file per command
e2e/                    # e2e tests (builds real binary, no mocks)
e2e/testdata/           # fixture directories for e2e tests
internal/config/        # Config struct, ~/.joca path resolution
version/                # version string, set via -ldflags at build time
```

## Key Conventions

- Use `RunE` (not `Run`) on cobra commands so errors cause non-zero exit
- The `cfg config.Config` package-level variable in `cmd/root.go` is shared by all `cmd/*.go` files
- Pass `config.Config` by value, not by pointer (it is small and set once)
- Errors should wrap with context: `fmt.Errorf("doing X: %w", err)`
- Import grouping: **stdlib / external / local**, each group separated by a blank line (enforced by goimports)
- `RootCmd` is exported so external test packages (e.g. `cmd_test`) can inspect registered subcommands

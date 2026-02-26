# jip

CLI tool for stacked PRs with jj (Jujutsu) and GitHub.

## Tech stack

- Go (latest stable)
- No runtime dependencies — single compiled binary
- Cross-platform: Linux, macOS, Windows

## Architecture

- `main.go` — entry point
- Commands use the cobra CLI framework
- GitHub API access via go-gh or go-github
- Auth: GH_TOKEN / GITHUB_TOKEN → gh CLI → OAuth device flow (cli/oauth)

## Testing

- Prefer high-level integration tests that simulate user interactions
- Avoid brittle unit tests that test implementation details
- Tests should exercise the CLI from the outside where possible
- Use test fixtures / temporary jj repos for testing

## Development instructions

- You MUST always use `jj` for version control! ONLY if it's not installed then
  you may fall back to Git.
- Conventional commit messages (feat:, fix:, refactor:, etc.)
- Each commit is a self-contained, atomic change

## Pre-commit checks

Before committing, always run all of these and fix any issues:

1. `go vet ./...`
2. `gofmt -l .` (reformat with `gofmt -w .` if needed)
3. `go tool -modfile=golangci-lint.mod golangci-lint run ./...`
4. `go test ./...` (unit tests)
5. `go test -tags=integration ./...` (integration tests)

## Code style

- Follow standard Go conventions (gofmt, go vet, golangci-lint)
- Keep functions small and focused
- Error messages should be actionable

# 02 — Security

Overall the attack surface is small: jip shells out to `jj` with argv arrays
(no shell), never logs the token, stores it with 0600 permissions, and only
talks to the GitHub API. The items below are hardening and one genuine
footgun (S1). Nothing here is an active vulnerability.

## S1 (P2) Token can be sent to a host chosen by an environment variable

**Where:** `cmd/send.go:303`.

`GITHUB_API_URL` redirects all REST and GraphQL calls, including the
`Authorization` header, while the token is always the github.com one. On a
GHES-hosted Actions runner this env var points at the enterprise host by
default. Resolution is decided: remove the env var handling
([01-correctness.md](01-correctness.md) C12).

## S2 (P2) No HTTP timeouts, no cancellation

**Where:** `internal/github/pr.go:73` (`http.DefaultClient`),
`internal/github/client.go:53` (go-github with `nil` client), every
`context.Background()` call, `exec.Command` in `internal/jj/runner.go`.

A stalled TCP connection hangs `jip send` forever, possibly after branches
were pushed and before PR bodies were updated. Ctrl-C kills the process
without letting the retry loop stop cleanly.

**Fix:**
1. One shared `*http.Client{Timeout: 60 * time.Second}` (or a transport with
   dial/response-header timeouts) passed to `gogithub.NewClient` and used for
   GraphQL.
2. Create a root context in `cmd/root.go` with `signal.NotifyContext(ctx,
   os.Interrupt, syscall.SIGTERM)`; thread it through `gh.Service`,
   `jj.Runner` (`exec.CommandContext`) and `retry.Do`.

**Verify:** httptest handler that sleeps longer than the timeout returns an
error instead of hanging; retry test asserting cancellation ends the sleep.

## S3 (P3) OAuth client secret embedded in the binary

**Where:** `cmd/auth_login.go:14-17`, `:42` (`DetectFlow`).

The comment already states this is acceptable, and `gh` does the same. Two
refinements:
- `DetectFlow` falls back to the web-app flow, which is the only reason the
  secret is needed. Using `flow.DeviceFlow()` exclusively removes the need to
  ship the secret at all and narrows what a third party can do with it
  (running a look-alike "jip" web flow). Device flow is what the docs already
  promise ("OAuth device flow").
- The `repo` scope is the minimum for creating PRs on private repos with a
  classic OAuth app; document that this is why it is requested.

**Verify:** `jip auth login` still works on a machine without a browser
(device flow prints a code).

## S4 (P3) Plaintext token file

**Where:** `internal/auth/config.go`.

`~/.config/jip/config.json` with 0600/0700 is in line with many CLIs. OS
keyring storage was considered and deferred. Cheap improvements:
- Document the file location and permissions in `docs/reference.md`.
- Add `jip auth logout` that deletes the host entry (small, and the natural
  counterpart of `login`; optional).
- On Windows the mode bits are ignored; mention that the file is only as
  protected as the user profile.

## S5 (P3) Repository-level config is trusted implicitly

**Where:** `internal/config/config.go:57-81`, `cmd/send.go:71-84`
(`sendConfigKeys`).

`.jip.toml` from a cloned repository can set `remote`, `upstream` (a URL),
`base`, `reviewer`, `rebase`. None of these can leak the token or push code to
a remote that is not already configured in the jj repo, so the risk is low,
but a surprising `upstream` or `base` from a repo file is hard to notice.
Restricting keys per file would break the legitimate case where a project
commits `upstream` for fork contributors.

**Fix:** print the effective config values that differ from defaults together
with their source file at the start of `send` (one line, e.g.
`Config: base=dev (.jip.toml), upstream=... (config.local.toml)`), and add a
short "trust" note to the configuration section of `docs/reference.md`.

## S6 (P3) GraphQL query built by string concatenation

**Where:** `internal/github/pr.go:128-141`.

Branch names are escaped for `\` and `"` only. Git ref rules exclude most
dangerous characters, so this is not exploitable today, but it is fragile.
Use GraphQL variables (`$b0: String!` … passed in `Variables`) and drop the
manual escaping. Also switch `Authorization: bearer` to the conventional
`Bearer` casing.

**Verify:** existing `pr_test.go` plus a case with a branch name containing a
quote.

## S7 (P3) CI and release supply chain

**Where:** `.github/workflows/ci.yml`, `release.yml`, `.goreleaser.yaml`.

- Add `permissions: contents: read` to `ci.yml` (currently inherits defaults).
- Pin actions to commit SHAs (Dependabot keeps them updated; the tag comment
  stays readable).
- Add a `govulncheck ./...` step (currently clean, keep it that way).
- Upgrade `go-github` from v68 to v80 manually; Dependabot cannot do major
  bumps. Check the changelog for the used surface: `PullRequests.Create/Edit`,
  `Issues.CreateComment`, `PullRequests.RequestReviewers`, `NewRequest/Do`,
  `ErrorResponse`, `RateLimitError`, `AbuseRateLimitError`.
- Optional: publish build provenance with `actions/attest-build-provenance`
  so `go install` users and release downloads can be verified.

## S8 (info) Things checked and found fine

- No token in debug output (`logCmd` prints jj argv only; slog calls log PR
  numbers and titles, not headers).
- `jj` is executed with an argument vector; user revsets never reach a shell.
- Token file and directory are created with 0600/0700.
- `govulncheck` reports no known vulnerabilities in the dependency graph.

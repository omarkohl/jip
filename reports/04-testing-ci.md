# 04 — Testing and CI

The integration suite is the project's real safety net: 50 scenarios in
`cmd/send_integration_test.go` plus jj-level tests, all against real jj repos
and an in-memory GitHub mock. It is well designed. The problem is that it does
not run where it matters.

## T1 (P1) CI silently skips every integration test

**Where:** `.github/workflows/ci.yml`, `release.yml`,
`cmd/send_integration_test.go:2156` (`checkJJ`),
`internal/jj/resolve_integration_test.go:302`.

`make precommit` runs `go test -tags=integration ./...`, but `ubuntu-latest`
has no `jj` binary and nothing in the workflows installs one. `checkJJ` calls
`t.Skip`, so CI is green while exercising none of `executeSend`. Releases are
cut on the same basis.

**Fix:**
1. Install jj in both workflows, pinned to a version (download the
   `x86_64-unknown-linux-musl` tarball from jj's GitHub release for the chosen
   version, verify its checksum, add to `PATH`). Add a second job or matrix
   entry for the minimum supported jj version once 03/A3 determines it.
2. Make the skip loud in CI: when `CI` is set (GitHub Actions sets it),
   `checkJJ` should `t.Fatal("jj not installed")` instead of skipping.
3. Add a step that prints `jj --version` so logs show what was tested.

**Verify:** the CI log shows integration test names running; temporarily
breaking `executeSend` fails CI.

## T2 (P2) No cross-platform CI despite cross-platform releases

Binaries ship for macOS and Windows; only Linux is ever tested. Risk areas:
`os.UserConfigDir`, path handling in `WorkspaceRoot`, temp-dir symlinks
(`/private/tmp` on macOS is already handled in one test), CRLF in jj output on
Windows, `t.Chdir` semantics.

**Fix:** matrix `ubuntu-latest`, `macos-latest`, `windows-latest` for unit
tests at minimum; jj publishes binaries for all three, so run the integration
suite there too once T1's install step is parameterised by OS. Allow the
Windows integration job to be `continue-on-error` initially and tighten once
green.

## T3 (P2) Nothing tests the CLI from the outside

**Where:** `cmd/send.go:208-340` (`runSend`), `cmd/send_config_test.go`.

CLAUDE.md asks for tests that "exercise the CLI from the outside where
possible", but every integration test calls `executeSend` directly with a
prepared `sendOpts`. Untested end-to-end: flag parsing and negations, config
file application in a real repo, auth resolution failure message, remote and
upstream resolution (name vs URL, `pushOwner` prefixing), `--stack`/`--no-stack`
reconciliation, exit codes.

**Fix:**
1. Add two package-level seams in `cmd`: `newGitHubService func(token,
   remoteURL string) (gh.Service, error)` and `resolveToken func(host string)
   (string, string)`. Production wires the real ones; tests swap in the mock.
2. Write tests that `t.Chdir` into a temp jj repo, set `rootCmd.SetArgs`,
   capture `SetOut`/`SetErr`, and call `rootCmd.Execute()`. Cover: `send
   --dry-run` default, a `.jip.toml` + `--no-draft` negation, `--upstream
   <name>` and `--upstream <url>`, missing auth, exit code on non-benign skip.
3. One true black-box smoke test: build the binary with `go build` in
   `TestMain` and run `jip version`, `jip send --help`, and `jip send` outside
   a repo, asserting exit codes and messages. Keep it under a second.

## T4 (P2) `internal/github/stacks.go` has no tests

200 lines against an undocumented API with subtle semantics (404 means
"feature disabled", `Unstack` returns 204 or 200-with-remaining, `FindStackForPR`
via query parameter). Add `httptest` coverage in the style of
`client_test.go` for each method, including the 404 path of `StacksEnabled`
and the not-fully-dissolved path of `Unstack`. Also missing: the 5xx retry and
GraphQL `errors` paths of `LookupPRsByBranch`.

## T5 (P1) Regression tests for the correctness report

| Item | Test |
|---|---|
| C1 | Unit: two bookmarks on one commit, explicit `-b` wins. Integration: cut `release` from `main`, send `-b release`, assert PR base. |
| C2 | Table test for `next.js`, `ssh://`, trailing slash, userinfo URLs. |
| C3 | Unit: 200 kB diff is truncated under 65,536 chars with notice. Integration: `CommentOnPR` error does not prevent body/marker update. |
| C4 | Unit: predicate; httptest 422 → exactly one request; rejected push of 3 bookmarks finishes quickly. |
| C7 | Rerun after simulated comment failure posts comment from marker; `--diff-since-push` uses remote head. |
| C8 | Description `"\n\nbody"` is pre-skipped with the no-description reason. |
| C10 | Corrupt token file: `SaveToken` returns an error, file untouched. |
| C12 | No test reads `GITHUB_API_URL`; client tests use the seam. |
| C13 | `--dry-run --rebase` leaves op log head and bookmarks unchanged. |

## T6 (P3) Test hygiene

- `send_integration_test.go` is 2,800 lines. Split by topic: mock service,
  helpers, basic send, skips, interdiff/comments, config/flags, native stacks.
- Prefer `t.TempDir()` over `os.MkdirTemp` + `RemoveAll`; keep the
  `JIP_KEEP_REPO` escape hatch in one shared helper used by both packages.
- `cmd/completion_integration_test.go:26-31` uses `os.Chdir` with manual
  restore; use `t.Chdir`.
- The suite takes about 93 s locally and is fully serial. Every test creates
  its own temp repo, so add `t.Parallel()` to tests that do not `Chdir`
  (most of them) and verify jj tolerates concurrent repos (it does; they are
  independent directories). Expect a 3-5× speed-up on multi-core machines.
- Several tests assert on substrings of human-readable output ("2 PR(s)
  sent"). Acceptable, but after the `Reporter` refactor (03/A1) prefer
  asserting on the Plan or mock state and keep output assertions to one
  dedicated test per message format.

## T7 (P3) Documentation of the test workflow

CONTRIBUTING's `JIP_KEEP_REPO` example targets `internal/jj` only; after T6
it applies everywhere. Add a line on running a single integration test in
`cmd` and on the CI jj version pin so contributors match it locally.

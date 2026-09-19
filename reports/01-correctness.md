# 01 — Correctness

Bugs confirmed by reading the code and, where marked, by running a throwaway
test. Ordered by priority. Each fix needs a regression test (see
[04-testing-ci.md](04-testing-ci.md) T5 for the list).

## C1 (P1) `ResolveBaseBranch` targets the wrong branch when bookmarks share a commit

**Where:** `internal/jj/resolve.go:12-47`.

**Problem:** the base revset is resolved to a commit ID and then the *first*
bookmark (alphabetical, jj's list order) whose remote target equals that commit
is returned. When two branches point at the same commit, the user's explicit
choice is ignored. Verified: with `main@origin` and `release@origin` at the same
commit, `ResolveBaseBranch(runner, "release", ...)` returns `"main"`. A freshly
cut release branch is exactly this situation, and `jip send -b release` then
opens every PR against `main` with no warning.

**Fix:**
1. If `revset` is literally the name of a known bookmark (or `name@remote`),
   return that name without resolving by commit.
2. Otherwise (e.g. `trunk()`, a change ID) keep the commit-based lookup, but
   when several bookmarks match, prefer one that is the resolved revset's own
   bookmark (`Change.Bookmarks` from the `jj log` output) before falling back
   to first-match, and print which one was chosen.

**Verify:** unit test with two bookmarks on one commit for both `-b release`
and `-b main`; integration test cutting `release` from `main` and sending with
`-b release`, asserting the PR base.

## C2 (P1) `ParseRepoFromURL` mangles dotted repo names and rejects `ssh://` URLs

**Where:** `internal/github/repo.go:10-11`.

**Problem:** both regexes end in `([^/.]+)`, so the repo name stops at the
first dot. Verified outputs:

| Input | Result |
|---|---|
| `https://github.com/vercel/next.js.git` | repo `next` |
| `git@github.com:vercel/next.js.git` | repo `next` |
| `ssh://git@github.com/owner/repo.git` | parse error |

The first two silently point the API client at a different repository (PR
lookups return nothing, PR creation fails with 404). The third is a common
form jj users write, and it aborts `send` at start-up.

**Fix:** replace the regexes with explicit handling: parse `scheme://` forms
with `net/url` (accept `https`, `http`, `ssh`, `git`, `git+ssh`), parse the
scp-like `user@host:owner/repo` form by splitting on the first `:`, take the
last two path segments, strip only a trailing `.git` and trailing `/`. Reject
anything with fewer than two segments. Consider whether go-gh's
`pkg/repository.Parse` already covers this before writing new code, but keep
the behaviour table above as the spec.

**Verify:** table-driven test covering the three rows above plus
`https://user:tok@github.com/o/r.git`, trailing slash, and `owner/repo.js`.

## C3 (P1) Oversized diff comment aborts the send half-way

**Where:** `cmd/send.go:930-992` (`postChangesComment`),
`internal/github/prbody.go:195` (`BuildDiffComment`).

**Problem:** GitHub rejects comment and PR bodies above 65,536 characters with
a 422. A large interdiff therefore fails `CommentOnPR`, which is retried three
times (see C4), then returned as an error. At that point the branches are
already pushed, but the PR bodies and pushed-commit markers of step 9 are never
written and later PRs in the loop are never created or updated. The rerun then
finds remote head == new commit and posts no comment at all (mitigated by C7).

**Fix:**
1. In `BuildDiffComment`, enforce a byte budget (about 60,000 to leave room
   for the footer). Emit whole file sections until the budget is hit, then a
   line such as `_Diff truncated: N more file(s), M lines. Use the links below
   to see the full diff._` and the footer. Same budget for
   `BuildStackedPRBody` on pathological commit messages.
2. Make comment failures non-fatal: print a warning to stderr, mark the change
   as changed, and continue. The body/marker update in step 9 must still run.

**Verify:** unit test with a 200 kB synthetic diff asserting length and the
truncation notice; integration test with a mock `CommentOnPR` that returns an
error, asserting the PR body still gets its marker.

## C4 (P1) Retry policy retries permanent failures and multiplies push waits

**Where:** `internal/retry/retry.go:43`, every method in
`internal/github/client.go` and `stacks.go`, `internal/jj/runner.go:185-219`
(`GitFetch`, `GitPush`), fallback loop in `cmd/send.go:694-732`.

**Problems:**
- REST calls retry on *every* error, including 4xx: a 422 "PR already
  exists", a 404, a 403 primary rate limit. Each adds about 3 s of sleep for
  nothing. `LookupPRsByBranch` already special-cases 5xx, so behaviour is
  inconsistent across the same client.
- `CommentOnPR` and `CreatePR` are not idempotent. A network error after the
  server processed the request causes a duplicate comment or PR on retry.
- `GitPush` is wrapped in retry, and the batch-failure fallback then pushes
  each bookmark individually, each again with three attempts. A legitimately
  rejected push of N bookmarks costs about 3 + 3N seconds of sleeping before
  the user sees the reason.
- Rate limiting is not handled: go-github's `RateLimitError` should fail fast
  with the reset time; `AbuseRateLimitError.RetryAfter` should be honoured.

**Fix:**
1. Give `retry.Do` a predicate: `retry.Do(ctx, fn, retry.If(isTransient))`
   or a `retry.Permanent(err)` wrapper that stops immediately. Keep the
   exponential backoff.
2. For go-github: transient = network errors, 5xx, `AbuseRateLimitError`
   (sleep `RetryAfter`). Permanent = any other `*ErrorResponse`,
   `RateLimitError` (return an actionable message with the reset time).
3. For jj push/fetch: retry only when the output looks like a network failure;
   do not retry when jj itself refuses (non-fast-forward, "Refusing",
   "rejected"). Do not wrap the per-bookmark fallback pushes in retry at all.
4. Make `retry` context-aware (sleep via `select` on `ctx.Done()`), see
   02 / S2.

**Verify:** unit tests for the predicate; httptest server returning 422 and
asserting exactly one request; a timing assertion that a rejected push of 3
bookmarks completes in well under 5 s.

## C5 (P2) `--rebase` can turn merged changes into empty PRs

**Where:** `internal/jj/runner.go:287` (`Rebase`), `cmd/send.go:397-402`.

**Problem:** after a PR is rebase-merged on GitHub, the local change still
exists with a different commit hash. `jj rebase -b X -d main` (without
`--skip-emptied`) leaves it as an empty commit, which jip then sends as a PR
with no diff. Resolution is the "skip empty changes" feature in
[05-features.md](05-features.md) F2; listed here because it is a correctness
hole in the documented `jip s --rebase` workflow.

## C6 (P3) Stack navigation becomes inconsistent when part of a stack is skipped

**Where:** `cmd/send.go:852-875` (step 9), `computeStackPRs` at `:998`.

**Problem:** only *active* PRs get their body rewritten, and their navigation
lists only active PRs. A conflicted change C on top of A and B leaves A and B
listing `#A, #B` while C's stale body still lists `#A, #B, #C`. Reviewers see
contradictory stacks.

**Fix:** include the PRs of skipped-but-existing changes in the navigation of
active PRs (they are still real PRs in the same dependency chain), while still
not touching the skipped PRs themselves. A short "(not updated in this
send)" note is optional. Reuse the DAG before filtering to compute the chain.

**Verify:** integration test with a conflicted tip; assert A's body still
references C's PR number.

## C7 (P1, decided) Use the pushed-commit marker as the default interdiff base

**Where:** `cmd/send.go:930-992`, flag definitions at `:48,53`,
`docs/reference.md` "Diffing against jip's last send".

**Problem:** the default interdiff base is the remote head *before* this push.
If a send fails after pushing (C3, a network error, Ctrl-C), the rerun sees
remote head == new commit and posts nothing, so reviewers never get the
"changes since" comment. Direct pushes by others distort it too. The marker
already solves both, but only behind `--diff-since-jip`.

**Decision:** marker by default. Implementation:
1. In `postChangesComment`, always try `ParsePushedCommit`, then
   `ParseReviewCommit`, then fall back to the remote head. Header reads
   "Changes since last jip send" when a record was used, "Changes since last
   push" otherwise (unchanged wording).
2. Add `--diff-since-push` (bool, configurable, with `--no-diff-since-push`
   negation) that forces the remote-head base. Deprecate `--diff-since-jip`
   and `--no-diff-since-jip` as no-ops via `MarkDeprecated`; keep accepting
   the config key with a deprecation warning for one release.
3. Update `sendConfigKeys`, `sendNegations`, `docs/reference.md` (flag table,
   config key list, the whole "Diffing against jip's last send" section, which
   should now describe the default and the opt-out).

**Verify:** adapt `TestIntegration_SendDiffSinceJip*` to the default; add a
test for `--diff-since-push`; add a test that a rerun after a simulated
comment failure still posts the comment using the marker.

## C8 (P3) A description with an empty first line produces an invalid PR update

**Where:** `cmd/send.go:467` (pre-skip check) and `:771` (title update).

**Problem:** the pre-skip only tests `TrimSpace(Description) == ""`. A
description like `"\n\nSome body"` passes, `Title()` returns `""`, creation
falls back to `jip: <id>` but every later send tries `UpdatePR` with an empty
title, which GitHub rejects.

**Fix:** pre-skip when `TrimSpace(c.Title()) == ""` with the same
"add a commit message" reason. Remove the `jip: <id>` fallback title, which
then becomes unreachable.

## C9 (P3) `Change.Body()` drops lines between a multi-line subject and the blank line

**Where:** `internal/jj/change.go:32-39`.

**Problem:** for `"title\nsecond line\n\nbody"` the body is `"body"`; the
second line is lost from the PR. Conventional single-line subjects are
unaffected, so this is low priority, but data loss is silent.

**Fix:** define body as everything after the first line, trimmed. Document in
`docs/why.md` or `reference.md` that the first line is the PR title and the
rest is the description.

## C10 (P3) `SaveToken` discards an unreadable token file

**Where:** `internal/auth/config.go:53-57`.

**Problem:** any `LoadConfig` error (including a parse error on an existing
file) is treated as "no config", so a successful login overwrites the file and
drops other hosts' tokens.

**Fix:** only treat `os.IsNotExist` as empty; return other errors with the
path in the message.

## C11 (P3) `--reviewer` is silently ignored for existing PRs

**Where:** `cmd/send.go:829-833`.

**Problem:** reviewers are requested only in the create branch. Users who add
`-r` when updating a PR get no reviewer and no message.

**Fix (choose one and document it in `reference.md`):** apply reviewers on
update too (idempotent for already-requested reviewers; re-requests review from
those who already reviewed, which is arguably wanted after changes), or print a
notice that reviewers apply only to newly created PRs. Recommendation: apply on
update.

## C12 (P2, decided) Disable the half-implemented GitHub Enterprise path

**Where:** `cmd/send.go:303-304`, `internal/github/client.go:47-70`,
`cmd/auth_status.go:30`, hard-coded `https://github.com` in
`internal/github/prbody.go:164,258` and the footer links.

**Problem:** `GITHUB_API_URL` switches the REST base URL, but the token is
always resolved for `github.com`, the GraphQL URL is derived as
`<api>/graphql` (GHES uses `/api/graphql`), and all rendered links point at
github.com. On a GHES Actions runner this env var is set to the GHES host, so
jip would send a github.com token to another host. The maintainer cannot test
GHES and does not intend to implement it.

**Decision:** disable, document, open an issue.
1. Stop reading `GITHUB_API_URL` in `runSend`. Keep the `apiURL` parameter of
   `NewClient` for tests only (or move it to an unexported test seam).
2. Add to `docs/reference.md` under Authentication: "jip supports github.com
   only. GitHub Enterprise Server is not supported; see issue #N."
3. Open a GitHub issue with `gh issue create` using this text:

   > **Title:** Support GitHub Enterprise Server
   >
   > jip currently supports github.com only. Supporting GHES needs: a `--host`
   > flag / `host` config key; resolving the token per host (go-gh's
   > `TokenForHost` already supports this); deriving REST (`/api/v3`),
   > GraphQL (`/api/graphql`) and web URLs from the host; using the host in
   > all links rendered into PR bodies and comments; and tests against a fake
   > host. The maintainer has no GHES instance to test against, so this is
   > open for contribution by someone who does.

**Verify:** grep confirms no `GITHUB_API_URL` outside tests; unit tests for
the client keep passing via the test seam.

## C13 (P2) `--dry-run` mutates the repository

**Where:** `cmd/send.go:386-402` (fetch, `--rebase`), `:568`
(`EnsureBookmarks` → `BookmarkSet`), dry-run exit at `:659`.

**Problem:** `--dry-run` is documented as "Show what would happen without
making changes", but before the dry-run exit jip runs `jj git fetch`, performs
the `--rebase`, and creates `jip/...` bookmarks. A user previewing
`jip s --rebase -n` gets their stack rebased.

**Fix:** skip `Rebase` and pass `createNew=false` to `EnsureBookmarks` in
dry-run, reporting would-be bookmark names instead. Keep the fetch (read-only
for local changes) but mention it in the flag help. Falls out naturally of
the plan/execute split in 03/A1 if that lands first.

**Verify:** integration test: `--dry-run --rebase` on a stack behind `main`
leaves the operation log head and bookmark list unchanged.

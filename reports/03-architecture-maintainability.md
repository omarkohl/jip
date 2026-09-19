# 03 — Architecture, maintainability, consistency

The package layout (`cmd`, `internal/jj`, `internal/github`, `internal/config`,
`internal/auth`, `internal/retry`) is sound and the boundaries are respected:
`internal/jj` knows nothing about GitHub and vice versa. The debt is
concentrated in one place: `cmd/send.go`.

## A1 (P2) `executeSend` is a 550-line function that mixes planning, execution and reporting

**Where:** `cmd/send.go:364-913`, helpers to `:1330`.

**Symptoms:**
- Skip bookkeeping lives in four parallel structures (`preSkipIDs`,
  `preSkippedChanges`, `skippedIDs`, `skippedStates`) with two near-identical
  printers (`printPreSkippedChanges`, `printAllSkipped`) and a counter that
  takes all of them (`nonBenignSkips`).
- Dry-run is an early `return` in the middle of the pipeline, so it cannot
  show what step 8/9 would do (title changes, base retargets, body updates),
  and the mutating steps before it still run (01/C13).
- Output formatting (`fmt.Fprintf(w, ...)`) is interleaved with decisions, so
  adding `--quiet` or `--json`, or reusing stages for `jip status`, means
  copying code.
- Pure graph logic (`computeStackPRs`, `stackGroups`, `checkLinearStacks`,
  native-stack reconciliation) sits in `cmd` and is only reachable through
  integration tests.

**Target shape (plan / execute):**

```
internal/send (or cmd/send/)
  resolve.go    ResolveStacks + pre-skips (private, empty description, empty diff)
  bookmarks.go  assign bookmarks, detect displaced/diverged, cascade skips
  plan.go       type Plan { Items []Item }; Item { Change, Bookmark, Action(create|update|skip), Reason, DesiredBase, DesiredTitle, DesiredBody }
  execute.go    push, create/update PRs, comments, bodies, native stacks
  report.go     Reporter interface: Info/Warn/Summary; text implementation
  skips.go      one skipSet type: add(change, reason), cascade(parents), nonBenign(), entries()
```

`send --dry-run` prints the Plan. `status` (05/F1) reuses `resolve` and
`bookmarks` with `createNew=false`. Tests can assert on the Plan without a
mock GitHub for the planning half.

**Approach:** do it incrementally, one commit per extracted stage, keeping the
integration suite green after each. Start with `skipSet` (pure refactor, no
behaviour change), then `Reporter`, then the Plan.

## A2 (P2) No context or cancellation; retry is not composable

Covered in [02-security.md](02-security.md) S2 and
[01-correctness.md](01-correctness.md) C4. Architecturally: `jj.Runner` and
`gh.Service` methods should take `context.Context` as the first parameter;
`retry.Do(ctx, fn, opts...)` with a retryable predicate.

## A3 (P2) Minimum jj version is undeclared and unchecked

**Where:** templates in `internal/jj/runner.go:15-38`, `jj interdiff --git`,
`bookmark list --quiet`, `COMPLETE=fish` in `cmd/completion.go`.

The README says "requires jj" with no version. Template functions like
`json()`, `local_bookmarks`, `normal_target`, `tracked`/`synced` and the
`interdiff` command each appeared in specific jj releases; on an older jj the
failure is an opaque template error.

**Fix:**
1. Determine the oldest jj release that passes the integration suite (run it
   against two or three older binaries from jj's releases page).
2. Document it in README "Requirements" and CONTRIBUTING.
3. At start-up (`workspaceRunner`), run `jj --version`, parse `jj X.Y.Z`, and
   fail with "jip requires jj >= X.Y, found A.B" before doing anything.
4. Test the minimum and latest versions in CI (see 04/T1).

## A4 (P3) `realRunner` repeats the same exec boilerplate in every method

**Where:** `internal/jj/runner.go:106-301`.

Every method builds args, calls `logCmd`, runs, captures output, logs, wraps
the error. Collapse into one `run(args ...string) (stdout, stderr string, err
error)` helper (plus a `runCombined` variant) and make each method a one-liner
around it. This also gives one place to add `--ignore-working-copy` (A5) and
the context (A2).

## A5 (P3) Every jj invocation re-snapshots the working copy

Read-only commands (`log`, `bookmark list`, `config get`, `interdiff`, the
`CommitExists` probe) can pass `--ignore-working-copy` once an initial
snapshot has happened. Benefits: faster on large trees, no surprise snapshot
of files the user edits while jip runs, and no "stale working copy" failures
in secondary workspaces. Keep one snapshotting command first (the initial `jj
log` is fine) so that a user-supplied `@` revset is current.

## A6 (P3) Dead and unused code

- `gh.Service.GetAuthenticatedUser` (`internal/github/client.go:19,161`):
  never called in production. Remove from the interface, the client and the
  mock.
- `gh.UpdatePROpts.Draft` (`client.go:83`): never set; go-github's REST edit
  cannot toggle draft anyway. Remove.
- `jj.SyncState`, `SyncWith`, `ChangeBookmark.SyncState`
  (`internal/jj/bookmark.go:11-85,195`): computed, never read by `send`; only
  `Conflict` and `Displaced` matter. Either drop them or use them for
  `status` (05/F1), which is the better outcome. Decide when building
  `status`.
- `PRInfo.State`, `PRInfo.IsDraft`: unused today, needed by `status`. Keep.

## A7 (P3) Linter configuration

`make lint` runs golangci-lint with defaults only (no `.golangci.yml`). Add
one enabling at least: `errorlint`, `gocritic`, `revive`, `misspell`,
`unparam`, `unconvert`, `nolintlint`, `gosec` (exclude G204 for the
intentional `exec.Command("jj", ...)`), `govet` with `shadow`. Fix what it
finds in a separate commit.

## A8 (P3) Config file naming is confusing

`~/.config/jip/config.json` holds tokens; `~/.config/jip/config.toml` holds
preferences. Same stem, different purpose. Rename the token file to
`hosts.json` (mirrors `gh`), reading the old name as a fallback for one or two
releases, and document both files in `docs/reference.md`.

## A9 (P3) Decision records

Several design decisions live only in FAQ prose and code comments: every PR
targets the base branch, jip owns only a marked section of the PR body
(05/F5), no local state, marker-by-default. Add `docs/adr/` with one short file per decision
(context, decision, consequences). The decisions taken in this review
(README list) are the first four entries.

---

## Consistency (small items, batch into one or two commits)

X1 (decided) **Document which parts of a PR jip overwrites.** The title and
the `jip:begin`/`jip:end` section are regenerated from the commit message on
every send; text outside the section is kept. Done as part of 05/F5.

X2 **Warnings go to stdout.** `cmd/send.go:438,795,831,967,1259` print
`warning:` lines to the same writer as progress output. Route them to stderr
(part of the `Reporter` in A1, or a second writer parameter in the meantime).

X3 **Two different "not authenticated" messages.** `cmd/send.go:274` and
`cmd/auth_status.go:27` word the same condition differently and only one
mentions `gh auth login`. Use one shared message constant.

X4 **`auth status` bypasses the shared client.** It builds its own go-github
client with no retry/timeout. After S2 it should use the same constructor so
timeouts apply everywhere.

X5 **Stale local permission entry.** `.claude/settings.local.json` allows
`make check`, which was renamed to `make precommit`.

X6 **CLAUDE.md says "go-gh or go-github".** Both are used: go-gh only for
token resolution, go-github for everything else. State that.

X7 **`docs/comparison.md` calls jip "Early".** The project is at v1.6 with a
stable feature set; reword or remove the maturity row.

X8 **Integration keep-repo helper differs between packages.** `internal/jj`
honours `JIP_KEEP_REPO`, `cmd` tests always `RemoveAll`. CONTRIBUTING
documents the env var as general. Make `cmd` honour it too (shared helper).

X9 **Error message style.** Most errors use ` — ` before the hint, some use
`. Run ...`. Pick one (the dash style dominates) and align the few outliers.

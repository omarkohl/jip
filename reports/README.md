# jip codebase review — 2026-09-18

Critical evaluation of jip at commit `8b67789` (after v1.6.0). Each report is
self-contained and written for an implementing agent: every item states the
problem, where it lives, why it matters, the intended fix, and how to verify.

| Report | Focus |
|---|---|
| [01-correctness.md](01-correctness.md) | Confirmed bugs and failure modes in `send` |
| [02-security.md](02-security.md) | Token handling, trust boundaries, supply chain |
| [03-architecture-maintainability.md](03-architecture-maintainability.md) | Structure of the send pipeline, dead code, consistency |
| [04-testing-ci.md](04-testing-ci.md) | CI gaps, test architecture, missing coverage |
| [05-features.md](05-features.md) | Agreed new features and explicitly deferred ideas |

## Decisions already made by the maintainer

These were discussed during the review and are settled. Do not re-open them.

1. **The pushed-commit marker becomes the default interdiff base.** The old
   remote-head behaviour stays available behind a `--diff-since-push` flag.
   See 01 / C7.
2. **jip owns only a marked section of the PR body.** The commit message is
   the source of the PR title and of everything between `jip:begin` /
   `jip:end` markers; text outside (PR template, manual edits, checklists) is
   preserved. See 05 / F5.
3. **GitHub Enterprise Server is out of scope.** Disable the half-working
   `GITHUB_API_URL` path, document github.com-only, and open a tracking issue
   with the drafted text. See 01 / C12.
4. **Features to build:** `jip status`, skipping empty changes, a command
   to fetch and track all open jip PRs when switching machines, and the owned
   PR body section with PR template support. Orphaned-PR
   cleanup, labels/assignees, and OS keyring storage are deferred. See 05.

## Priorities

| Priority | Meaning |
|---|---|
| P1 | Wrong behaviour users can hit today, or a gap that hides such bugs |
| P2 | Robustness, maintainability or coverage debt that will keep costing |
| P3 | Nice to have; do when touching the area anyway |

Suggested order of work: 04/T1 (CI runs no integration tests) first because
every other change should be protected by it; then 01 P1 items; then the
pipeline refactor in 03/A1 before building the features in 05, since `status`
and `track` reuse the resolve/bookmark stages.

## Working conventions (from CLAUDE.md, repeated for agents)

- Use `jj`, never `git`, for version control. One coherent change per commit,
  conventional commit messages, focus on *why*.
- Run `make precommit` before committing; `make fmt` to format.
- Prefer high-level integration tests through the CLI over unit tests of
  implementation details; use temporary jj repos.
- Keep docs in `docs/` in sync with behaviour changes (`reference.md` is the
  flag/config reference).

## Verified facts the reports rely on

- Unit suite: passes in under 1 s. Integration suite (`-tags=integration`):
  passes locally in about 93 s with jj 0.45.1 and Go 1.27.1.
- `make lint` is clean with golangci-lint v2.13.2 default linters (no config
  file present).
- `govulncheck ./...` reports no vulnerabilities.
- go-github is pinned at v68; v80 is current. Dependabot cannot bump major
  versions because the import path changes.

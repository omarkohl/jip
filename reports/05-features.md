# 05 — Features

Agreed with the maintainer. Build them after the pipeline refactor
([03](03-architecture-maintainability.md) A1) so `resolve` and `bookmarks`
stages can be reused; if the refactor is postponed, extract just those two
stages first.

## F1 (P2) `jip status` — read-only view of the stack and its PRs

**Goal:** answer "what is the state of my stack on GitHub?" without side
effects. Today the only way is `jip send --dry-run`, which fetches and
creates bookmarks.

**CLI:** `jip status [revsets...]` (alias `st`). Same revset default (`@-`)
and `--base`, `--remote`, `--upstream` handling as `send`. Flags:
`--fetch` (run `jj git fetch` first; default off so the command is pure),
`--all` (later: every open jip PR of the user, see F3). No config keys beyond
the shared ones.

**Data:**
- Resolve stacks and assign bookmarks with `createNew=false` (no
  `jj bookmark set`).
- Extend the GraphQL lookup to return `state`, `isDraft`, `reviewDecision`,
  `mergeable`, `commits(last:1){nodes{commit{statusCheckRollup{state}}}}`,
  `url`, and search states `[OPEN, MERGED, CLOSED]` ordered by `UPDATED_AT`
  so a merged PR is reported as merged rather than "no PR".
- Reuse `SyncWith` (currently dead code, 03/A6) for the local-vs-remote
  column.

**Output** (one line per change, bottom of stack first, one blank line
between stacks):

```
  #12  open   approved  checks:pass   synced     wysxkqnm  feat: add user data model
  #13  draft  review    checks:fail   ahead      pknrltvo  feat: add user store
  --   --     --        --            local-only qnvzmtrp  feat: add user API endpoint
```

Plus the same skip reasons `send` would report (conflict, no description,
private, empty). Exit code 0 unless a jj/API call fails; status is
informational.

**Tests:** integration tests with the mock service covering created, merged,
and missing PRs and a displaced bookmark; assert that no bookmark is created
and no API mutation happens (mock counters).

## F2 (P2) Skip empty changes; make `--rebase` drop emptied commits

**Problem:** after a PR is merged, the local change survives with a new hash.
`jip send --rebase` (or a manual `jj rebase`) leaves an empty commit that jip
happily turns into an empty PR. See [01](01-correctness.md) C5.

**Design:**
1. Add `empty` to `logTemplate` (`",\"empty\":" ++ if(empty, "true",
   "false")`) and `Empty bool` to `jj.Change`.
2. Pre-skip empty changes as **benign** with reason `empty change (no diff)
   — probably already merged; abandon it or add content`. No cascade:
   descendants remain sendable because the empty commit is still pushed as
   part of their history. But an *existing* PR of an emptied change should be
   reported so the user can close it: reason `empty change — its PR #N is
   probably merged; close it or abandon the change`.
3. `Rebase` passes `--skip-emptied` so jj abandons commits that become empty
   during `--rebase`. Surface jj's "Abandoned N commits that became empty"
   line in the output. This mutates history slightly more aggressively than
   today, but it is jj's standard post-merge behaviour and is recoverable with
   `jj undo`; document it in `docs/reference.md` under `--rebase`.
4. **Exempt merge commits.** jj reports a clean merge (no conflict
   resolution) as `empty`, so a naive skip would drop the "Merging main into
   release" workflow in `docs/workflows.md`, whose PR *is* such a merge
   commit. Only pre-skip changes that are empty and have exactly one parent
   (add `parents.len()` to the template, or use `empty && !merges()` in the
   revset). Add integration tests that the release-merge and private-merge
   batch workflows still behave the same.

**Tests:** integration: merged-then-rebased change is skipped benignly and
`send` exits 0; `--rebase` abandons the emptied commit; a stack with an empty
middle commit still sends its descendants.

## F3 (P2) Fetch and track all open jip PRs (machine switching)

**Goal:** on a second machine, get every open PR the user sent with jip as a
local, tracked bookmark so `jj log` shows them and `jip send` can update them.
Today this requires knowing branch names and running `jj bookmark track`
manually for each.

**CLI:** `jip track [--remote origin] [--upstream ...] [--all-authors]`
(alias candidates: `pull`, `fetch`; avoid `fetch` because it suggests plain
`jj git fetch`). Default: PRs authored by the authenticated user whose head
branch starts with `jip/`.

**Design:**
1. GraphQL on the PR repository (upstream when set):
   `pullRequests(states: OPEN, first: 100, after: $cursor, orderBy: {field:
   UPDATED_AT, direction: DESC}) { nodes { number url headRefName author {
   login } headRepository { nameWithOwner } title } }`, paginated. Filter
   client-side: `author.login == viewer.login` (fetch `viewer { login }` in the
   same query), `headRefName` has prefix `jip/`, and, for forks,
   `headRepository.nameWithOwner` equals the push remote's owner/repo.
2. `jj git fetch --remote <push remote>` once (the remote branches arrive as
   `name@remote`).
3. For each branch not already tracked: `jj bookmark track name@remote`.
   Skip and report bookmarks that are already tracked or that exist locally
   pointing elsewhere (would conflict).
4. Print a table: PR number, bookmark, title, and `tracked` / `already
   tracked` / `skipped (reason)`.
5. Mutations are limited to jj bookmark tracking; nothing is written to
   GitHub. Add `--dry-run`.

**Runner additions:** `BookmarkTrack(name, remote string) error`; the fetch
already exists.

**Tests:** integration with two jj repos sharing one bare remote: repo A sends
a stack (mock records PRs with head refs), repo B runs `track` and asserts
the bookmarks exist and are tracked (`jj bookmark list --all-remotes`
template), then `jip send` from B updates the same PRs.

**Docs:** new section in `docs/workflows.md` ("Continuing on another
machine") and the command table in `reference.md`.

## F4 (P1, decided) Marker as default interdiff base

Specified in [01](01-correctness.md) C7; listed here because it changes
user-facing behaviour and docs.

## F5 (P2, decided) jip-owned PR body section; PR template support

**Problem:** every send replaces the whole PR body with the commit message
(`cmd/send.go:858-870`). Projects that require a PR template lose it (GitHub
does not apply templates to API-created PRs), checkbox ticks made on GitHub are
wiped, and a manually opened PR that jip adopts (bookmark with an existing PR,
`cmd/send.go:561-566`) loses its description.

**Design:** jip owns one delimited section at the **bottom** of the body.
Everything above it belongs to humans and is never touched.

```
<PR template, adopted PR's description, or anything edited on GitHub>

<!-- jip:begin (generated from the commit message; edits here are overwritten) -->

…stack header, commit body, footnote (BuildStackedPRBody output)…

<!-- jip:pushed-commit=<hash> -->
<!-- jip:end -->
```

1. **Title:** always overwritten from the commit, including adopted PRs.
2. **Create:** outside text = the repo's PR template if one exists, else
   empty. Look up the single-file locations GitHub supports
   (`.github/`, repo root, `docs/`; filename `pull_request_template.md`,
   case-insensitive) in the base revision via `jj file show`, not the working
   copy. The multi-template `PULL_REQUEST_TEMPLATE/` directory is ignored. The
   template is only used at creation; afterwards the outside text is the
   user's.
3. **Update:** parse the current body:
   - exactly one `jip:begin` followed by one `jip:end` → replace the section,
     keep everything outside byte-for-byte;
   - no section markers but a `jip:pushed-commit` marker → legacy jip body,
     replace the whole body with the new section (no template);
   - no markers at all → manually opened PR: keep the whole body as outside
     text, append the section;
   - anything else (unpaired/duplicate/misordered markers) → replace the
     whole body with just the new section. Recoverable from GitHub's edit
     history; no warning machinery needed.
4. **Pushed-commit marker** moves inside the section. `ParsePushedCommit`
   and `ParseReviewCommit` read only the section when markers are present
   (so pasted old bodies outside cannot confuse them) and the whole body
   for legacy bodies. `WithPushedCommitMarker`/`stripPushedCommitMarkers`
   become part of the section renderer. Keeps C7 working.
5. Single (non-stacked) PRs get the section too.
6. Blank lines around every marker so GitHub renders the Markdown between
   them instead of treating it as a raw HTML block.
7. Skip the update API call when the resulting body equals the current one
   (as today).

**Where:** new section build/parse helpers in `internal/github/prbody.go`;
`cmd/send.go` create/update paths; template lookup via the jj runner.

**Tests:** unit tests for the parser/merger (all four update cases, marker
inside the section only, round-trip idempotence). Integration with the mock
service: template applied on create; a checkbox ticked on GitHub survives a
re-send; adopted manual PR keeps its description and gets its title
overwritten; legacy body is replaced without duplication; broken markers
replace the body.

**Docs:** `docs/reviewing.md` and `docs/workflows.md` FAQ (what jip
overwrites: title and its section; where to put checklists: above the
section), `docs/reference.md` body format.

---

## Deferred or rejected (do not implement without re-asking)

- **Orphaned PR cleanup / `jip prune`** — deferred. A safe subset would be a
  hint in `status --all` listing open jip PRs whose change ID is unknown
  locally, without closing anything.
- **`--label`, `--assignee`, OS keyring token storage** — deferred.
- **GitHub Enterprise Server** — out of scope; disable and open an issue
  (01/C12).
- **`jip auth logout`** — small and reasonable, not requested; fine to add
  when touching `auth`.

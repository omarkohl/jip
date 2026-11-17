# Command reference

## Commands

| Command | Description |
|---|---|
| `jip auth login` | Authenticate with GitHub using OAuth device flow |
| `jip auth status` | Show current authentication status |
| `jip completion` | Generate shell auto-completion scripts |
| `jip help` | Display help about a command |
| `jip send` (alias: `s`) | Create or update PRs for a stack of changes |
| `jip version` | Display the version |

Global flags:

| Flag | Short | Default | Description |
|---|---|---|---|
| `--debug` | | | Enable debug logging to stderr (also via `JIP_DEBUG` env var) |
| `--help` | `-h` | | Display help (same as `help` command) |
| `--version` | `-v` | | Display the version (same as `version` command) |

## `send` flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--base` | `-b` | `trunk()` | Base branch (defaults to the repo's trunk branch, usually `main`) |
| `--remote` | | `origin` | Push remote name |
| `--bookmark-policy` | | `auto` | `auto`: reuse eligible bookmarks and generate missing names; `require`: never generate names |
| `--exclude-bookmarks` | | `normal_target.immutable()` | Boolean jj bookmark-template expression selecting bookmarks that must not be used |
| `--upstream` | `-u` | | Upstream remote name or URL (where PRs are opened). Bare `--upstream` means the `upstream` remote; pass a value with `=` (`--upstream=fork`) |
| `--dry-run` | `-n` | | Show what would happen without making changes |
| `--overwrite-pr-body` | | | Overwrite PR bodies with broken markers or legacy jip bodies; does not bypass code or bookmark safety checks |
| `--reviewer` | `-r` | | Add reviewers (repeatable, comma-separated) |
| `--draft` | `-d` | | Create PRs as drafts |
| `--existing` | `-x` | | Only update PRs that already exist (skip new ones) |
| `--stack` | | `default` | Stacking mode: `default` (stack navigation in PR descriptions), `gh-native` (GitHub's native stacked PRs), or `none` (send only the tip of each stack as a single PR) |
| `--no-stack` | | | Deprecated — use `--stack=none` |
| `--rebase` | | | Rebase the stack onto the base branch before sending |
| `--diff-since-jip` | | | Diff against jip's own last send (recorded in the PR) instead of the current remote head |
| `--no-change-comment` | | `default` | Comment posted when an updated PR has no code changes: `default`, `short`, or `none` |
| `--no-draft` | | | Create PRs ready for review, ignoring a configured `draft` |
| `--no-rebase` | | | Don't rebase before sending, ignoring a configured `rebase` |
| `--no-diff-since-jip` | | | Diff against the current remote head, ignoring a configured `diff-since-jip` |
| `--no-upstream` | | | Open PRs on the push remote, ignoring a configured `upstream` |
| `--no-reviewer` | | | Don't request reviewers, ignoring a configured `reviewer` |

## PR body ownership and templates

jip always sets PR titles from the commit subject, including PRs opened by
someone else. The commit body and generated navigation go inside one section:

```html
<!-- jip:begin (generated from the commit message; edits here are overwritten) -->

Generated navigation and commit body

<!-- jip:pushed-commit=<hash> -->

<!-- jip:end -->
```

The section is appended at the bottom when first added. Later sends replace
only that section. Text before **and after** a valid section is preserved
byte-for-byte. Keep checklists and GitHub-only notes outside the section.
Standalone PRs and all stacking modes use the same ownership boundary.
Do not put reserved jip comments in templates or commit bodies, except as
examples inside fenced code blocks.
Close all Markdown code fences before sending. An unclosed fence can hide
section markers, so jip treats it as a body conflict. Preserve is not possible
until the fence is closed; Overwrite, Skip, and Abort remain available.

For a new PR, jip reads `pull_request_template.md` (case-insensitive filename)
from the configured committed stack base, not the working copy. The search
order is `.github/`, repository root, then `docs/`. Template directories are
ignored. Multiple filename case variants in the selected directory are an
error. The template is outside the owned section and is applied only once.
Forks use the stack base on the named upstream remote; URL-only upstreams
must be changed to a named remote to create PRs. Missing templates are allowed;
an unavailable revision or a template read error stops the send before pushing.

Broken, duplicate, or misordered section markers and legacy jip bodies require
a decision before pushing. A legacy body has a pushed-commit marker or an old
generated review link, but no section boundary. In a terminal, choose:

- **Preserve:** keep the old text, remove recognized jip metadata comments, and
  append a new section. Old descriptions and navigation can remain duplicated.
- **Overwrite:** replace the whole description with a generated section.
- **Skip:** skip this PR and its dependent changes. Independent changes proceed.
  Skipped body conflicts cause a non-zero exit.
- **Abort** (default): stop without pushing or changing PRs.

Without a terminal, body conflicts stop the send with an error. Pass `--overwrite-pr-body`
to choose Overwrite for these conflicts. This flag does not delete outside text
around a valid section, and cannot be set in config. Dry runs do not prompt;
use `--dry-run --overwrite-pr-body` to preview a conflicting-body replacement.

jip reads the body again immediately before updating it. A new conflict leaves
the body unchanged and reports that the code was already pushed. An earlier
decision, including `--overwrite-pr-body`, does not authorize overwriting later edits.
There is still a small interval between the final read and write in which a
concurrent edit can be lost. Updates whose bodies are unchanged are not sent.
Commit records for `--diff-since-jip` are read only inside a valid section;
legacy bodies remain readable until migration. Broken sections are not trusted.

## Configuration files

Workflow preferences can be set persistently instead of being passed as flags
on every invocation. jip reads two TOML files, each of which may have a
`.local.` sibling holding machine-specific overrides you don't want to share:

1. **Global** — `~/.config/jip/config.toml` (the platform's user config dir,
   e.g. `$XDG_CONFIG_HOME/jip/config.toml`), then
   `~/.config/jip/config.local.toml`
2. **Repo** — `.jip.toml` in the repository root (commit it to share team
   defaults), then `.jip.local.toml`

Later files override earlier files; CLI flags override all config values. So a
more specific location always wins, and a `.local.` file overrides its own
sibling.

The `.local.` files are for settings that shouldn't be shared: personal
overrides alongside a `config.toml` tracked in your dotfiles, or a deviation
from a team default without dirtying the committed `.jip.toml`. **Add
`.jip.local.toml` to your `.gitignore`** — jip does not do this for you.

Keys mirror the `send` flag names: `base`, `remote`, `upstream`, `draft`,
`bookmark-policy`, `exclude-bookmarks`,
`stack`, `no-stack`, `rebase`, `diff-since-jip`, `reviewer`,
`no-change-comment`.
Per-invocation flags (`--dry-run`, `--existing`) and the `--no-` cancelling
flags below cannot be set from config.

```toml
# ~/.config/jip/config.toml — personal preferences
rebase = true
diff-since-jip = true
```

```toml
# ~/.config/jip/config.local.toml — machine-specific overrides
upstream = "git@github.com:my-fork/project.git"
```

```toml
# .jip.toml (repo root) — team defaults
base = "dev"
draft = true
reviewer = ["alice", "team/backend"]
```

```toml
# .jip.local.toml (repo root, gitignored) — your overrides for this repo
draft = false
```

### Overriding config for a single invocation

Every configured value can be cancelled from the command line. Flags that carry
a value are overridden by passing the value you want (`--base main`,
`--stack default`, `--no-change-comment none`). The remaining flags have a
`--no-` counterpart, since there is no value to pass:

| Config key | Cancel with |
|---|---|
| `draft` | `--no-draft` |
| `rebase` | `--no-rebase` |
| `diff-since-jip` | `--no-diff-since-jip` |
| `upstream` | `--no-upstream` |
| `reviewer` | `--no-reviewer` |

```bash
# ~/.config/jip/config.toml sets rebase = true, but not this time
jip send --no-rebase
```

A flag and its `--no-` counterpart cannot be combined.

Two flags start with `no-` without being negations: `--no-change-comment` takes
a value (`default`, `short` or `none`), and the deprecated `--no-stack` is an
alias for `--stack=none`. Cancel a configured `stack` or `no-stack` by passing
`--stack` with the mode you want.

## Revsets

`send` takes optional revset arguments to select which changes to send. The
default is `@-` (the parent of the working copy). Multiple revsets are combined
with OR and resolved against the base branch.

```bash
jip send              # send @- and its ancestors up to base
jip send @--          # send only the grandparent change
jip send @- xyz       # send changes reachable from @- or xyz
```

## Base branch (`--base` / `-b`)

The default `trunk()` picks up your repo's trunk branch automatically —
typically `main`, but also `master` or `trunk` depending on the repo.

Pass a branch name to override:

```bash
jip send -b develop         # target the "develop" branch
jip send -b release/2026    # target a release branch
```

The base must exist as a bookmark on the push/upstream remote — it's the
branch your PRs target on GitHub.

## Branch naming and bookmark selection

Revsets select changes; bookmarks select their PR branches. Argument order does
not define stack order: jip uses the commit graph, including multiple stacks.
For each change, jip selects an eligible local bookmark in this order:

1. A bookmark explicitly named in the command arguments.
2. A bookmark with an open PR.
3. Any other existing local bookmark.

Several bookmarks at the winning priority are an error, not an alphabetical
choice. Name exactly one bookmark for that change to resolve the ambiguity.
Other open PRs on the change are reported and left untouched. Remote-only and
conflicted bookmarks cannot be explicitly selected; remote-qualified selections
must agree with the local target.

If no bookmark exists, `auto` generates `jip/<description-slug>/<change-id>`.
Projects that require developer-supplied names can prohibit generation:

```toml
# .jip.toml
bookmark-policy = "require"
```

This does not validate names or reject existing `jip/` bookmarks. Missing and
ambiguous assignments are reported together before bookmark creation, rebase,
pushes, or PR changes. `--existing` still means existing **PRs** only; it skips
changes without PRs rather than requiring bookmarks for them.

The base bookmark is always excluded. By default, bookmarks targeting jj
immutable commits are also excluded, following the repository's
`immutable_heads()` alias. This is local jj policy, not GitHub branch protection.
Replace the exclusion predicate using jj's **bookmark-template syntax**, not
revset syntax. For example:

```toml
exclude-bookmarks = 'normal_target.immutable() || name.starts_with("release/")'
# Or disable the configurable exclusion (the base is still excluded):
# exclude-bookmarks = 'false'
```

Name predicates apply to individual bookmarks, even when an eligible feature
bookmark shares a commit with an excluded release bookmark. Explicit selection
does not bypass exclusions. Invalid expressions stop the send.

### Read-only send plan

`jip send --dry-run` runs the send logic without fetching, creating bookmarks,
pushing, or modifying PRs. It prints generated bookmark names, push targets,
new PR heads/bases/draft state, title/body updates, comments, reviewer requests,
and native-stack operations. New PR and stack numbers use negative placeholders;
links containing them will use the real numbers during a send.

The plan uses current local and cached remote-tracking state. Remote changes,
push checks, GitHub permissions, and merge-queue restrictions can affect an
actual send. It does not promise that remote operations will succeed. jj reads
can still snapshot working-copy files as usual.

Dry-run rejects `--rebase`, including a configured rebase. Rebase with jj first,
then use `jip send --dry-run --no-rebase` to preview the resulting stack.

## Fork-based workflow

jip works with fork-based workflows. You don't need push access to the upstream
repository. Use `--upstream` to specify where PRs should be opened while pushing
branches to your fork.

```bash
# Assuming your fork is "origin" and you want to open a PR in the upstream project
jj git remote add upstream https://github.com/some/project.git
jip send --upstream   # bare flag defaults to the "upstream" remote

# or another remote, or a URL without adding a remote (note the "=")
jip send --upstream=https://github.com/some/project.git
```

Arguments separated from `--upstream` by a space are revsets, not flag values.
Use `=` to select another remote, even if a bookmark has the same name.

## Rebasing before send (`--rebase`)

Use `--rebase` to rebase the stack onto the base branch before pushing. This
ensures PRs don't contain stale diffs when the base branch has moved forward.

```bash
jip send --rebase
```

This is equivalent to running `jj rebase` manually before `jip send`, but
saves a step.

## Stacking modes (`--stack`)

By default, jip creates one PR per commit, all targeting the base branch, and
renders stack navigation into each PR description. `--stack` selects other
modes:

### GitHub native stacked PRs (`--stack=gh-native`)

Uses GitHub's built-in stacked-PRs feature (private preview — the repository
must be enrolled via <https://gh.io/stacksbeta>). Each PR targets the branch
of the change below it, and jip links the PRs into a native GitHub stack via
the Stacks API — the same thing `gh stack link` does, without needing the `gh`
CLI. GitHub's own UI then provides stack navigation, atomic merge-down, and
server-side rebasing, so jip puts the commit message body in the owned section
without adding stack navigation.

```bash
jip send --stack=gh-native      # or set stack = "gh-native" in .jip.toml
```

Notes:

- Adding changes on top of a stack appends to the existing GitHub stack. The
  Stacks API is append-only, so after reordering, removing, or inserting
  changes mid-stack, jip dissolves the stack and recreates it (the PRs and
  their reviews are untouched — only the stack association is rebuilt).
- Sending only part of a stack (a narrower revset, or changes skipped for
  conflicts) leaves the PRs above untouched: the GitHub stack is kept as is.
- Stacks cannot span forks, so `--stack=gh-native` cannot be combined with
  `--upstream`.
- Branching (non-linear) stacks cannot be expressed; jip fails with an error.
- If the repository is not enrolled in the preview, jip fails before making
  any changes.

### Single PR for a stack (`--stack=none`)

Use `--stack=none` to bundle an entire linear stack into a single PR using the
tip commit's bookmark and description. This is useful when all commits in the
stack were already reviewed elsewhere (e.g. merging `main` into the `release`
branch).

```bash
jip send --stack=none
```

The deprecated `--no-stack` flag is an alias for `--stack=none`.

## Diffing against jip's last send (`--diff-since-jip`)

When you update a PR, jip posts a "Changes since last push" comment showing what
changed. By default the comparison is made against the current remote head. If
someone pushes to the branch directly (without jip), that remote head moves and
the comparison no longer reflects what reviewers last saw through jip, producing
an incomplete diff.

`--diff-since-jip` instead compares against the commit jip itself last sent.
jip records that commit in an invisible marker in the PR body on every send, so
the base is recovered automatically — the comment header reads "Changes since
last jip send":

```bash
jip send --diff-since-jip
```

If the recorded commit isn't available locally (for example it was pushed from
another machine and you haven't fetched it), jip can't compute the diff and
instead posts a note saying so rather than a misleading one. jip writes the
marker on every send (including `--stack=none` sends), and because each PR carries
its own marker, this works across an entire stack. A PR only lacks a marker if
jip has never sent it — for instance a PR created outside jip, or one last sent
by a jip version predating this feature. In that case jip falls back to
comparing against the remote head and the comment header reads "Changes since
last push", not "Changes since last jip send".

## No-change comments (`--no-change-comment`)

When an updated PR contains no code changes (e.g. it was only rebased), jip
posts a comment noting that. That signal distinguishes a jip send from a
direct force-push made outside jip, but repeat occurrences add noise for
reviewers. `--no-change-comment` controls the behavior:

- `default` — the usual formatted comment ("No code changes …")
- `short` — a single plain-text line: `No changes since last push.`
- `none` — no comment at all

```bash
jip send --no-change-comment=short
```

Like other workflow preferences, this can be set persistently in a
[config file](#configuration-files).

## Authentication

jip uses the following authentication methods, in order:

1. `GH_TOKEN` or `GITHUB_TOKEN` environment variable
2. `gh` CLI authentication (if `gh` is installed and authenticated)
3. Built-in OAuth device flow (`jip auth login`)

jip currently supports github.com only. GitHub Enterprise Server is not supported yet; contributions to add GHES support are encouraged (see issue #49). `GITHUB_API_URL` is ignored so a GHES Actions runner cannot send a github.com token to another host. A remote whose host is not github.com is rejected with an actionable error instead of calling api.github.com.

## Shell Completion

Execute `jip completion --help` to learn how to generate different shell
completions.

This also includes auto-completing jj revsets and bookmark names where
appropriate.

For example, for Bash you can add the following to `~/.bashrc`:

```bash
source <(jip completion bash)
```

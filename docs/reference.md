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
| `--upstream` | `-u` | | Upstream remote name or URL (where PRs are opened) |
| `--dry-run` | `-n` | | Show what would happen without making changes |
| `--reviewer` | `-r` | | Add reviewers (repeatable, comma-separated) |
| `--draft` | `-d` | | Create PRs as drafts |
| `--existing` | `-x` | | Only update PRs that already exist (skip new ones) |
| `--stack` | | `default` | Stacking mode: `default` (stack navigation in PR descriptions), `gh-native` (GitHub's native stacked PRs), or `none` (send only the tip of each stack as a single PR) |
| `--no-stack` | | | Deprecated — use `--stack=none` |
| `--rebase` | | | Rebase the stack onto the base branch before sending |
| `--diff-since-jip` | | | Diff against jip's own last send (recorded in the PR) instead of the current remote head |
| `--no-change-comment` | | `default` | Comment posted when an updated PR has no code changes: `default`, `short`, or `none` |

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
`stack`, `no-stack`, `rebase`, `diff-since-jip`, `reviewer`,
`no-change-comment`.
Per-invocation flags (`--dry-run`, `--existing`) cannot be set from config.

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

## Fork-based workflow

jip works with fork-based workflows. You don't need push access to the upstream
repository. Use `--upstream` to specify where PRs should be opened while pushing
branches to your fork.

```bash
# Assuming your fork is "origin" and you want to open a PR in the upstream project
jj git remote add upstream https://github.com/some/project.git
jip send --upstream upstream

# or without adding a remote
jip send --upstream https://github.com/some/project.git
```

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
server-side rebasing, so jip leaves PR descriptions as plain commit messages.

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

## Shell Completion

Execute `jip completion --help` to learn how to generate different shell
completions.

This also includes auto-completing jj revsets and bookmark names where
appropriate.

For example, for Bash you can add the following to `~/.bashrc`:

```bash
source <(jip completion bash)
```

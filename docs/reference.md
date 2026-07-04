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
| `--no-stack` | | | Send only the tip of each stack as a single PR |
| `--rebase` | | | Rebase the stack onto the base branch before sending |
| `--diff-since-jip` | | | Diff against jip's own last send (recorded in the PR) instead of the current remote head |

## Configuration files

Workflow preferences can be set persistently instead of being passed as flags
on every invocation. jip reads two TOML files:

1. **Global** — `~/.config/jip/config.toml` (the platform's user config dir,
   e.g. `$XDG_CONFIG_HOME/jip/config.toml`)
2. **Repo** — `.jip.toml` in the repository root (commit it to share team
   defaults)

Repo values override global values; CLI flags override both.

Keys mirror the `send` flag names: `base`, `remote`, `upstream`, `draft`,
`no-stack`, `rebase`, `diff-since-jip`, `reviewer`. Per-invocation flags
(`--dry-run`, `--existing`) cannot be set from config.

```toml
# ~/.config/jip/config.toml — personal preferences
rebase = true
diff-since-jip = true
```

```toml
# .jip.toml (repo root) — team defaults
base = "dev"
draft = true
reviewer = ["alice", "team/backend"]
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

## Single PR for a stack (`--no-stack`)

By default, jip creates one PR per commit. Use `--no-stack` to bundle an
entire linear stack into a single PR using the tip commit's bookmark and
description. This is useful when all commits in the stack were already reviewed
elsewhere (e.g. merging `main` into the `release` branch).

```bash
jip send --no-stack
```

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
marker on every send (including `--no-stack` sends), and because each PR carries
its own marker, this works across an entire stack. A PR only lacks a marker if
jip has never sent it — for instance a PR created outside jip, or one last sent
by a jip version predating this feature. In that case jip falls back to
comparing against the remote head and the comment header reads "Changes since
last push", not "Changes since last jip send".

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

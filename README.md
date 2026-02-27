# jip

**jip** is a CLI tool for managing stacked pull requests using
[jj (Jujutsu)](https://github.com/jj-vcs/jj) and GitHub.

Each commit is a self-contained, atomic unit of change that gets its own pull
request. When you update a PR, jip posts the diff as a comment so reviewers can
see exactly what changed since their last review.

## What are stacked PRs?

A stacked PR workflow breaks large changes into a chain of small, reviewable
pull requests where each PR builds on the previous one. Instead of a single
monster PR that touches dozens of files, you create a sequence:

```
PR 3: Add API endpoint (depends on PR 2)
PR 2: Add database migration (depends on PR 1)
PR 1: Add data model
```

Each PR is small and focused, making review faster and more thorough. Reviewers
can approve and merge from the bottom up.

## Why jip?

### The commit-based workflow

Most Git workflows center around branches. You create a feature branch, pile
commits onto it, and open a PR for the whole branch. Commit messages become
throwaway ("wip", "fix tests", "address review") because the branch is the unit
of review.

jip takes the opposite approach: **the commit is the unit of review**. Each
commit should be a high-quality, self-contained change. Each commit gets its own
PR. jj makes this workflow natural — rewriting, reordering, and splitting
commits is a first-class operation, not a painful interactive rebase. jip
bridges this to GitHub's PR model.

This approach is inspired by [Gerrit](https://www.gerritcodereview.com/)'s
review model, adapted for GitHub.

### The force-push problem

When you update a PR by force-pushing, GitHub shows the full diff against the
base branch but provides no good way to see *what changed since the last
review*. Reviewers are left re-reading the entire PR or manually diffing
commits.

jip solves this by posting the output of `jj interdiff` as a comment on the PR
every time it is updated. This shows reviewers exactly what changed in the
commit's patch, with parent/rebase changes factored out. This is possible
because jj tracks the evolution of each change via its change ID.

### No write access required

jip works with fork-based workflows. You don't need push access to the upstream
repository. Use `--upstream` to specify where PRs should be opened while pushing
branches to your fork.

### No special merge process

PRs created by jip are normal GitHub PRs. You merge them through the GitHub UI,
`gh pr merge`, or however you normally merge PRs. There is no separate "land" or
"submit" command. After merging, rebase locally with jj as usual, then run jip
again to update the remaining stack.

### Automatic bookmarks

Unlike some tools that require you to manually create jj bookmarks before
submitting, jip automatically creates and manages bookmarks for your changes.
You just point it at a set of changes and it handles the rest.

### Merge commits in stacks

jip supports stacked PRs where the revision graph contains merges — useful
when your stack includes changes that merge branches together.

## Usage

### Basic workflow

```
# Make your changes as a stack of jj commits
# ... edit files ...
jj commit -m "feat: add data model"
# ... edit files ...
jj commit -m "feat: add migration"

# Create/update PRs for the stack (default revset: @-)
jip send

# After review feedback, edit a commit
jj edit <change-id>
# ... make changes ...

# Update the PRs (posts interdiff comments automatically)
jip send

# After bottom PR is merged upstream
jj git fetch
jj rebase -o main

# Update remaining PRs
jip send
```

### Commands

| Command | Description |
|---|---|
| `jip send` (alias: `s`) | Create or update PRs for a stack of changes |
| `jip auth login` | Authenticate with GitHub using OAuth device flow |
| `jip auth status` | Show current authentication status |

### `send` flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--base` | `-b` | `main` | Base branch |
| `--remote` | | `origin` | Push remote name |
| `--upstream` | `-u` | | Upstream remote name or URL (where PRs are opened) |
| `--dry-run` | `-n` | | Show what would happen without making changes |
| `--reviewer` | `-r` | | Add reviewers (repeatable, comma-separated) |
| `--draft` | `-d` | | Create PRs as drafts |
| `--existing` | `-x` | | Only update PRs that already exist (skip new ones) |
| `--no-stack` | | | Send only the tip of each stack as a single PR |

### Revsets

`send` takes optional revset arguments to select which changes to send. The
default is `@-` (the parent of the working copy). Multiple revsets are combined
with OR and resolved against the base branch.

```bash
jip send              # send @- and its ancestors up to base
jip send @--          # send only the grandparent change
jip send @- xyz       # send changes reachable from @- or xyz
```

### Fork-based workflow

```bash
# Push to your fork, open PRs against upstream
jip send --upstream upstream
```

### Single PR for a stack (`--no-stack`)

By default, jip creates one PR per commit. Use `--no-stack` to bundle an
entire linear stack into a single PR using the tip commit's bookmark and
description. This is useful when all commits in the stack were already reviewed
elsewhere (e.g. merging `main` into the `release` branch).

```bash
jip send --no-stack
```

## Requirements

- [jj (Jujutsu)](https://github.com/jj-vcs/jj) — jip does **not** work with
  Git directly
- A GitHub repository

## Authentication

jip uses the following authentication methods, in order:

1. `GH_TOKEN` or `GITHUB_TOKEN` environment variable
2. `gh` CLI authentication (if `gh` is installed and authenticated)
3. Built-in OAuth device flow (`jip auth login`)

## Installation

jip is distributed as a single compiled binary with no runtime dependencies.

```
go install github.com/omarkohl/jip@latest
```

Pre-built binaries for Linux, macOS, and Windows are available on the
[releases page](https://github.com/omarkohl/jip/releases).

## Comparison with existing tools

### vs [jj-spr](https://github.com/LucioFranco/jj-spr)

jj-spr is the most feature-rich existing tool. Key differences:

| | jip | jj-spr |
|---|---|---|
| Reviewer interdiff | Posted as PR comment via `jj interdiff` | Append-only branches (new commits added remotely) |
| Merge process | Normal GitHub merge | Requires `jj spr land` |
| Write access to target repo | Not required (fork workflow) | Required |
| Branch overhead | One branch per PR | Extra branches for append-only history |

jj-spr's append-only approach means the remote branch accumulates commits that
don't match your local history, which prevents merging via the normal GitHub UI.
jip keeps remote branches in sync with your local commits and solves the
interdiff problem through PR comments instead.

### vs [jj-stack](https://github.com/keanemind/jj-stack)

| | jip | jj-stack |
|---|---|---|
| Distribution | Compiled binary | npm package |
| Bookmark management | Automatic | Requires manual bookmark creation |
| Reviewer interdiff | Yes (PR comments) | No |
| Commits per PR | One (enforced) | Flexible |

### vs [jj-ryu](https://github.com/dmmulroy/jj-ryu)

jj-ryu is Graphite-inspired and supports both GitHub and GitLab. Key
differences:

| | jip | jj-ryu |
|---|---|---|
| CLI workflow | Run `jip send` on a set of changes | `ryu track` + `ryu submit` |
| Reviewer interdiff | Yes (PR comments) | No |
| Maturity | Early | Alpha |
| Commits per PR | One (enforced) | Flexible |

### vs [fj](https://github.com/lazywei/fj)

fj is a minimal Go tool with a similar philosophy (one commit per PR).

| | jip | fj |
|---|---|---|
| Reviewer interdiff | Yes (PR comments) | No |
| Bookmark management | Automatic | Manual |
| Fork workflow | Supported | Not documented |

### vs Git-based tools (ghstack, spr, Graphite)

These tools do not work with jj. jip is jj-native and does not support Git
directly. If you use Git, look at those tools instead.

| | jip | ghstack | spr (Git) | Graphite |
|---|---|---|---|---|
| VCS | jj only | Git only | Git only | Git only |
| Write access to target repo | Not required | Required (no fork support) | Required | Required |
| Merge process | Normal GH UI | `ghstack land` | `git spr merge` | Own merge flow |
| Reviewer interdiff | PR comments | Partial (no force-push) | No | Via SaaS |

## Releasing

Releases are automated with [GoReleaser](https://goreleaser.com/) via GitHub
Actions. Tag a version and push:

```bash
jj tag v0.1.0
git push --tags
```

This runs the full check suite, then builds binaries for Linux, macOS, and
Windows (amd64 + arm64) and publishes a GitHub Release.

## Development

Run `make` to see available targets:

```bash
make              # list targets
make build        # build the binary
make check        # run all checks (lint + tests)
make test         # unit tests
make test-integration  # integration tests (require jj)
```

Build with a specific version (for releases):

```bash
make build VERSION=0.2.0
```

### Integration test tips

```bash
# Verbose output (shows jj log, resolved DAGs with parent info)
go test -tags integration -v ./...

# Keep test jj repos for manual inspection
JIP_KEEP_REPO=1 go test -tags integration -v -run TestIntegration_OverlappingRevsets ./internal/jj/
# The repo path is printed in the test output, e.g.:
#   repo: /tmp/jip-integration-1234567890
# You can then inspect it:
#   jj -R /tmp/jip-integration-1234567890 log -r ::
```

## License

[MIT](LICENSE)

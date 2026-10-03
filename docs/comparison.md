# Comparison with other tools

**Key differentiator:** jip does not require write access to the target
repository — it works equally well for contributors submitting PRs from a fork.
Every other jj tool listed here pushes PR branches (and stack base branches) to
the target repository, so it requires write access. This makes them unsuitable
for open-source contribution workflows where you don't own the repo.

## vs [jj-spr](https://github.com/jennings/jj-spr)

jj-spr is the most feature-rich existing tool. Key differences:

| | jip | jj-spr |
|---|---|---|
| Reviewer interdiff | Posted as PR comment via `jj interdiff` | Append-only branches (new commits added remotely) |
| Merge process | Normal GitHub merge | `jj spr land`, then manual rebase |
| Write access to target repo | Not required (fork workflow) | Required (documented) |
| Branch overhead | One branch per PR | Extra base branches for dependent PRs |
| GitHub native stacks | Optional (`--stack=gh-native`) | Not supported |

jj-spr's append-only approach means the remote branch accumulates commits that
don't match your local history; they are squashed into one commit on landing.
jip keeps remote branches in sync with your local commits and solves the
interdiff problem through PR comments instead.

## vs [jj-stack (bos)](https://github.com/bos/jj-stack)

jj-stack (bos) is an actively developed Python tool with a similar
philosophy (one change per PR, managed branches).

| | jip | jj-stack (bos) |
|---|---|---|
| Distribution | Compiled binary | Python package (Python 3.14+) |
| Bookmark management | Automatic | Automatic (hidden `jj-stack/` branches) |
| Reviewer interdiff | Yes (PR comments) | Yes ("Revision history" comment with links) |
| Commits per PR | One (enforced) | One (enforced) |
| Merge process | Normal GitHub merge | `jj-stack merge`, or GitHub merge + `jj-stack sync` |
| GitHub native stacks | Optional (`--stack=gh-native`) | Always, for stacks of 2+ changes |
| Write access to target repo | Not required | Required (fork workflow explicitly unsupported) |

## vs [jj-stack (keanemind)](https://github.com/keanemind/jj-stack)

| | jip | jj-stack (keanemind) |
|---|---|---|
| Distribution | Compiled binary | npm package |
| Bookmark management | Automatic | Requires manual bookmark creation |
| Reviewer interdiff | Yes (PR comments) | No |
| Commits per PR | One (enforced) | Flexible |
| Write access to target repo | Not required | Required |

## vs [jj-ryu](https://github.com/dmmulroy/jj-ryu)

jj-ryu is Graphite-inspired and supports both GitHub and GitLab. Key
differences:

| | jip | jj-ryu |
|---|---|---|
| CLI workflow | Run `jip send` on a set of changes | `ryu track` + `ryu submit` |
| Reviewer interdiff | Yes (PR comments) | No |
| Maturity | Early | Alpha |
| Commits per PR | One (enforced) | Flexible |
| Write access to target repo | Not required | Required |

## vs [fj](https://github.com/lazywei/fj)

fj is a minimal Go tool with a similar philosophy (one commit per PR). It has
had no updates since 2023.

| | jip | fj |
|---|---|---|
| Reviewer interdiff | Yes (PR comments) | No |
| Bookmark management | Automatic | Manual |
| Write access to target repo | Not required | Required for stacks (bases are branches in the target repo) |

## vs Git-based tools (ghstack, spr, Graphite)

These tools do not work with jj. jip is jj-native and does not support Git
directly. If you use Git, look at those tools instead.

| | jip | ghstack | spr (Git) | Graphite |
|---|---|---|---|---|
| VCS | jj only | Git only | Git only | Git only |
| Write access to target repo | Not required | Required (no fork support) | Required | Required |
| Merge process | Normal GH UI | `ghstack land` | `git spr merge` | Own merge flow |
| Reviewer interdiff | PR comments | Partial (no force-push) | No | Via SaaS |

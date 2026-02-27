# Comparison with other tools

## vs [jj-spr](https://github.com/LucioFranco/jj-spr)

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

## vs [jj-stack](https://github.com/keanemind/jj-stack)

| | jip | jj-stack |
|---|---|---|
| Distribution | Compiled binary | npm package |
| Bookmark management | Automatic | Requires manual bookmark creation |
| Reviewer interdiff | Yes (PR comments) | No |
| Commits per PR | One (enforced) | Flexible |

## vs [jj-ryu](https://github.com/dmmulroy/jj-ryu)

jj-ryu is Graphite-inspired and supports both GitHub and GitLab. Key
differences:

| | jip | jj-ryu |
|---|---|---|
| CLI workflow | Run `jip send` on a set of changes | `ryu track` + `ryu submit` |
| Reviewer interdiff | Yes (PR comments) | No |
| Maturity | Early | Alpha |
| Commits per PR | One (enforced) | Flexible |

## vs [fj](https://github.com/lazywei/fj)

fj is a minimal Go tool with a similar philosophy (one commit per PR).

| | jip | fj |
|---|---|---|
| Reviewer interdiff | Yes (PR comments) | No |
| Bookmark management | Automatic | Manual |
| Fork workflow | Supported | Not documented |

## vs Git-based tools (ghstack, spr, Graphite)

These tools do not work with jj. jip is jj-native and does not support Git
directly. If you use Git, look at those tools instead.

| | jip | ghstack | spr (Git) | Graphite |
|---|---|---|---|---|
| VCS | jj only | Git only | Git only | Git only |
| Write access to target repo | Not required | Required (no fork support) | Required | Required |
| Merge process | Normal GH UI | `ghstack land` | `git spr merge` | Own merge flow |
| Reviewer interdiff | PR comments | Partial (no force-push) | No | Via SaaS |

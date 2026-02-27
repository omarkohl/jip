# Why commit-based workflow?

Most Git workflows center around **branches**. You create a feature branch,
pile commits onto it, and open a PR for the whole branch. Commit messages
become throwaway ("wip", "fix tests", "address review") because the branch is
the unit of review — nobody looks at individual commits.

jip takes the opposite approach: **the commit is the unit of review**.

## Commits are the permanent record

Pull requests are ephemeral. Once merged, they fade into GitHub's archive. But
commits stay in the repository forever. When someone runs `git log` or
`jj log` six months from now to understand *why* a line was changed, the commit
message is often the only source of context.

This makes polishing commits valuable work, not busywork:

- **A good commit message explains *why***, not just what. It captures the
  reasoning, trade-offs, and constraints that informed the change.
- **Small, focused commits** are easier to review, easier to revert if
  something goes wrong, and easier to cherry-pick for a hotfix release.
- **Clean history** lets you use tools like `bisect` effectively. When every
  commit is a self-contained change that passes tests, finding the commit that
  introduced a bug becomes mechanical.

jj makes this workflow natural. Rewriting, reordering, and splitting commits
is a first-class operation — not a painful interactive rebase. jip bridges
this to GitHub's PR model by turning each commit into its own PR.

## The force-push problem

When you update a PR by force-pushing, GitHub shows the full diff against the
base branch but provides no good way to see *what changed since the last
review*. Reviewers are left re-reading the entire PR or mentally diffing
commits.

jip solves this by posting the output of `jj interdiff` as a comment on the
PR every time it is updated. This shows reviewers exactly what changed in the
commit's patch, with parent/rebase changes factored out. This is possible
because jj tracks the evolution of each change via its change ID.

## Inspired by Gerrit

This approach is inspired by [Gerrit](https://www.gerritcodereview.com/)'s
code review model — where the commit has always been the unit of review —
adapted for GitHub's PR-based workflow.

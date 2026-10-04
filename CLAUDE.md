# Working on whynot

Follow CONTRIBUTING.md, imported below: building and testing, issues and pull
requests, comments, package documentation, modules and releases. See
[ARCHITECTURE.md](ARCHITECTURE.md) for how the code is organized.

@CONTRIBUTING.md

## Workflow

As a maintainer, changes go through a branch in this repository and a pull request, not
straight to `main`:

1. Create a branch off an up-to-date `main`.
2. Commit on the branch.
3. Push it and open a pull request with `gh pr create`.
4. Iterate on the pull request as needed.
5. Merge it with `gh pr merge --squash --delete-branch`, then
   `git checkout main && git pull`.

A trivial fix (a typo, a one-liner) may go straight to `main`.

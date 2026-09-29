# Working on whynot

See [ARCHITECTURE.md](ARCHITECTURE.md) for how the code is organized.

## Workflow

Changes go through a branch and a pull request, not straight to `main`:

1. Create a branch off an up-to-date `main`.
2. Commit on the branch.
3. Push it and open a PR with `gh pr create`: a short summary plus a test plan.
4. Iterate on the PR as needed.
5. Merge it with `gh pr merge --squash --delete-branch`, then
   `git checkout main && git pull`.

A trivial fix (a typo, a one-liner) may go straight to `main`.

A feature or a design change starts with a GitHub issue describing it,
before any code. Its PRs reference the issue: "Part of #N" when the work
spans several PRs (tracked as a task list in the issue), and "Closes #N"
on the one that completes it.

## Commit messages and PR titles

Prefix them with a [Conventional Commits](https://www.conventionalcommits.org/)
type: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`, `ci:`.
GoReleaser groups release notes by these prefixes (see `.goreleaser.yml`),
and a squash merge uses the PR title as the commit message, so the PR title
needs the prefix too.

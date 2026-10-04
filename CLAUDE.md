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

## Modules

The repository holds two Go modules: the core (`github.com/arnodel/whynot`,
the root) and the Gio backend (`backends/giobackend`, with its programs).
`go.work` at the root makes the Gio module use the local core while
developing; `./...` from the root covers the core only, so build and test
the Gio module from its own directory too (CI does both).

A backend added to this repository goes under `backends/`. If its framework
hasn't reached version 1, it gets its own module, as `giobackend` has, so
the framework's breaking changes can't force breaking changes on the core.

`go install` and users ignore `go.work`: the Gio module's `go.mod` must
require a core version that has everything it uses. Before tagging the
Gio module (`backends/giobackend/vX.Y.Z`), tag the core if needed, then
`go get github.com/arnodel/whynot@<that version>` and `go mod tidy` in
`backends/giobackend`, and check it builds with `GOWORK=off`.

## Commit messages and PR titles

Prefix them with a [Conventional Commits](https://www.conventionalcommits.org/)
type: `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`, `ci:`.
GoReleaser groups release notes by these prefixes (see `.goreleaser.yml`),
and a squash merge uses the PR title as the commit message, so the PR title
needs the prefix too.

## Comments

Write doc comments for someone *using* the function or type, not for
someone reconstructing how it was built.

- Keep: one or two sentences on what it is or does, the non-obvious
  details a caller needs to use it correctly, and the *why* when there's
  an obvious alternative someone could reasonably prefer.
- Cut: anything already visible in the signature or body; lists of
  callers ("used by X and Y" - grep finds them); history ("this used to
  do X") beyond a clause of why the current shape was chosen; design
  narratives (alternatives explored, how a value was derived), which
  belong in the commit message or PR description.
- Don't refer to things a reader of the source can't see, such as
  conversations or notes outside the repository.

Inline comments explain *why* a piece of code is the way it is, not what
the next lines do.

Contract packages (`canvas`, `images`, `codeblocks`, `fonts`) are the
exception to brevity where precision is needed: their doc comments are
the specification implementers work from.

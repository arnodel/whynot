# Contributing to whynot

Thank you for your interest in whynot! This page explains how the project is worked on.
[ARCHITECTURE.md](ARCHITECTURE.md) explains how the code is organized and why.

## Building and testing

whynot needs Go 1.25 or later. The repository holds two Go modules: the core, at the
root, and the Gio backend, in [`backends/giobackend`](backends/giobackend) together with
its programs. The `go.work` file at the root puts both in one workspace, so the Gio module
builds against your local copy of the core.

Run the checks that CI runs, for both modules:

```bash
gofmt -l .                              # lists any file that needs formatting
go vet ./... && go test ./...           # the core
cd backends/giobackend && go vet ./... && go test ./...
```

`./...` from the root covers the core only, which is why the Gio module needs its own
run. On Linux, Gio needs a few system libraries: see
[Gio's installation guide](https://gioui.org/doc/install/linux), or the list in
[`.github/workflows/tests.yaml`](.github/workflows/tests.yaml).

## Proposing a change

- **A feature or a design change starts with an issue** describing it, before any code,
  so that its shape can be discussed first. A bug fix can go straight to a pull request.
- **A pull request** has a short summary of what changes and why, and a test plan: what
  was checked, and how. If it belongs to an issue, it says so: "Part of #N" when the work
  spans several pull requests, or "Closes #N" for the one that completes it.
- **Pull request titles and commit messages** start with a
  [Conventional Commits](https://www.conventionalcommits.org/) type: `feat:`, `fix:`,
  `refactor:`, `docs:`, `test:`, `chore:` or `ci:`, with `!` for a breaking change
  (`feat!:`). Pull requests are squash-merged, so the title becomes the commit message,
  and the release notes group commits by these types.

## Writing comments

Write doc comments for someone *using* the function or type, not for someone
reconstructing how it was built.

- Keep: one or two sentences on what it is or does, the non-obvious details a caller
  needs to use it correctly, and the *why* when there's an obvious alternative someone
  could reasonably prefer.
- Cut: anything already visible in the signature or body; lists of callers ("used by X
  and Y": grep finds them); history ("this used to do X") beyond a clause of why the
  current shape was chosen; design narratives (alternatives explored, how a value was
  derived), which belong in the commit message or the pull request.
- Don't refer to things a reader of the source can't see, such as conversations or notes
  outside the repository.

Inline comments explain *why* a piece of code is the way it is, not what the next lines
do.

Contract packages (`canvas`, `input`, `images`, `codeblocks`, `fonts`) are the exception
to brevity where precision is needed: their doc comments are the specification that
implementers work from.

## Writing package documentation

- **Each public package has a `doc.go`** with its package comment: what the package is
  for, how it fits with the rest of whynot, and, where it helps, a short example. An
  example must compile as written, because readers paste it.
- **Concepts that span several types** get a section of the package comment (a line
  starting with `# `), such as the Coordinates section of package `whynot`.
- **Use doc links** (`[View]`, `[View.ScrollBy]`, `[canvas.Canvas]`) where a name first
  appears in a comment. A short package name works when some file of the package imports
  that package. Struct fields can't be linked: link the type and name the field.
- **Doc comments have no inline code syntax**: backquotes turn into quotation marks. Code
  goes in an indented block.

## Modules and releases

This section is for maintainers.

A backend added to this repository goes under `backends/`. If its framework hasn't
reached version 1, it gets its own module, as `giobackend` has, so that the framework's
breaking changes can't force breaking changes on the core.

`go.work` only applies inside this repository: `go install` and users ignore it. So the
Gio module's `go.mod` must require a version of the core that has everything it uses.
Before tagging the Gio module (`backends/giobackend/vX.Y.Z`):

1. Tag the core, if the Gio module needs changes that aren't in a tagged version yet.
2. In `backends/giobackend`, run `go get github.com/arnodel/whynot@<that version>` and
   `go mod tidy`.
3. Check that it builds outside the workspace: `GOWORK=off go build ./...`.

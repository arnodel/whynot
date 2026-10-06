---
name: api-page
description: Build a one-page HTML reference of whynot's public API (every public package's go doc, with what's new, changed or removed since a release marked) and publish it as an artifact. Use when asked for the API page, an API read-through or review page, or "go doc of every package on one page".
---

`.claude/skills/api-page/main.go` renders every public package with `go/doc`, much as
pkg.go.dev does: the package doc, then each exported name's declaration and doc comment.
Doc links point within the page, and links to other packages go to pkg.go.dev. It also
compares each declaration with a baseline release and marks it new or changed, showing
the old declaration for changed ones and listing removed names at the end of their
package. The page's HTML and CSS are in `head.html` and `tail.html`, embedded into the
program. All paths below are relative to the repository root.

## Steps

1. Check out the baseline release, usually the latest tag, somewhere outside the
   repository, such as the scratchpad:

   ```bash
   git worktree add --detach <scratchpad>/baseline v0.6.0
   ```

2. Generate the page:

   ```bash
   go run ./.claude/skills/api-page \
     -old <scratchpad>/baseline -baseline v0.6.0 \
     -commit $(git rev-parse --short HEAD) \
     -removed images \
     -out <scratchpad>/whynot-api.html
   ```

   `-removed` lists, comma-separated, the directories of packages that exist in the
   baseline but not now, so the page can name them. Leave it out if there are none.

3. Check that every in-page link has a target. This should print 0:

   ```bash
   cd <scratchpad>
   comm -23 <(grep -o 'href="#[^"]*"' whynot-api.html | sed 's/href="#//;s/"//' | sort -u) \
            <(grep -o 'id="[^"]*"' whynot-api.html | sed 's/id="//;s/"//' | sort -u) | wc -l
   ```

4. Publish the file with the Artifact tool. To update the existing page and keep its link,
   read it first and then publish with its `url`:
   https://claude.ai/artifact/JL4qjwCXpJUmKcYSvzQLr8. The "Read" checkboxes are stored in
   the viewer's browser, per artifact, so updating the page keeps them.

5. Remove the checkout when the read-through is done:

   ```bash
   git worktree remove <scratchpad>/baseline
   ```

## Maintenance

- **The package list is in `main.go`** (`pkgs`), in reading order and with each package's
  group: Core, Contracts, Ready-made, Backends. Update it when a public package is
  added, moved or removed. Compare with
  `go list ./... | grep -v '/internal\|/examples\|/cmd'`, plus `backends/giobackend`,
  which is its own module.
- **Only declarations are compared.** A name whose doc comment changed, but not its
  declaration, isn't marked. Say so when presenting the page, if docs changed a lot.
- Names declared in one `const` or `var` group share one entry, and each name gets an
  anchor.

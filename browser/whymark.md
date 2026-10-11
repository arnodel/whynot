# whymark

*Version 1, draft.*

whymark is a format for Markdown documents that run. A whymark document reads like any
Markdown document in any viewer; given to a whymark engine, it makes pages, one at a time,
in response to what a reader does.

This specification has three parts: the **document** (what an author writes), the
**request** (what a user agent sends), and the **response** (the page it gets back). It
says nothing about how pages are shown: that's up to the user agent, whether a browser, a
shell script, a test or an AI agent.

## The model

A document is a program, and a page is its output. A **turn** is one exchange: a request
in, a page out. The user agent reads the page, chooses one of its links, and sends the next
request.

Given a request and the document's content, the page is determined. So going back shows a
page kept from before, with nothing run again; following a link on an earlier page carries
on from that page, as if nothing after it had happened; and a list of turns, played again,
gives the same pages.

## Documents

A whymark document is Markdown with front matter: YAML between two `---` lines, at its
very start.

````markdown
---
title: The Cellar
whymark:
  version: 1
---

A tiny adventure. Prose before the first passage is for whoever reads the source.

```lua
function init(turn) return { lamp = false } end
```

# Hall

You're in a hall. `= turn.state.lamp and "Your lamp glows." or "It's dark below."`

`if not turn.state.lamp then`
- [Take the lamp](#take-the-lamp)
`end`
- [Go down](#cellar)

# (Take the lamp)

```lua
turn.state.lamp = true
```

You take the lamp from its hook. [Back to the hall](#hall)

# Cellar

`if turn.state.lamp then`
A dusty bottle of port. **You win!**
`else`
It's pitch black. [Back up](#hall)
`end`
````

### Front matter

- `title`: the document's title. `description`: a sentence about it, for listings.
- `whymark`: its presence makes the document a whymark document. Under it:
  - `version`: the version of this format the document is written for: `1`. An engine
    shows a document written for a version it doesn't know as an error.
  - `start`: the id of the passage the document starts at. By default, the first.
  - `journal`: `true` hints that the document reads best as a trail of pages, each below
    the one before. User agents may ignore it.

### Passages

Each level-1 heading starts a **passage**, and its id is the heading's anchor, as GitHub and
most Markdown tools make it: `# Take the lamp` is `take-the-lamp`. So read in a plain
viewer, a document is a gamebook whose links work. A passage's page shows its heading,
unless the heading is in brackets: `# (Take the lamp)` is a **quiet** passage, with the
same id, whose heading isn't shown. Headings within a passage use level 2 and below.

Before the first passage, `lua` blocks are the **prelude**, which runs at the start of every
turn; anything else there is ignored.

In a passage:

- A code span with one backquote runs: `` `= expr` `` writes the value of `expr` (nothing for
  `nil`); anything else is Lua statements, such as `` `if turn.state.lamp then` ``. A line
  holding only statement spans disappears, its newline too.
- A code span with two or more backquotes is shown, as code. So is any code block other
  than the ones below.
- A `lua` block runs where it is, and isn't shown.
- A link to `#id` goes to the passage `id`; a link to `?query` stays on the passage, with
  the query.

### The turn

Lua code sees the current turn as `turn`, passed to it, never global:

| | |
|---|---|
| `turn.state` | the trail's data, a table the turn may change; plain data only (nil, booleans, numbers, strings, and tables of those) |
| `turn.query` | the request's query, as a table of strings |
| `turn.input` | the fields typed in, by name, as a table of strings |
| `turn.passage` | the id of the passage shown |
| `turn.write(...)` | writes its arguments to the page |
| `turn.write_headers{title = …}` | writes the page's header (see below) |
| `turn.link(text, query)` | a Markdown link to `?query`, from a table, escaped |
| `turn.go(id)` | stops this passage and shows passage `id` instead; the page's location follows |
| `turn.show(id)` | runs passage `id`, without its heading, where it's called, then carries on |

`init(turn)`, if defined, returns the starting state, on a turn that starts the document;
without it, the state starts as an empty table. `page(turn)` writes the page. A document
with passages gets a `page(turn)` showing the turn's passage, and can't define its own;
a document without passages must define it: its body is then usually one `lua` block.

**The header** is written once, before anything else: `turn.write_headers{…}` writes it
with the fields given. Its only field, so far, is `title`, which defaults to the passage's
heading (shown or quiet), or for a document without passages, to the front matter's
`title`. In a document without passages, `turn.write` before `turn.write_headers` is an
error. In a passage, the first output writes the header itself, with the defaults, unless
the passage wrote it first; whitespace written before then doesn't count, so a `lua` block
after a passage's blank lines can still write the header, or go elsewhere. `turn.go` after
the page has started is an error.

**The sandbox:** the Lua standard libraries base, string, table, math, utf8 and coroutine,
and nothing else: no files, no network. A turn's CPU and memory are limited.

**A failed turn**, whether the document doesn't compile or its code raises an error, gives
a page all the same: the error, and the state the turn started from.

## Requests

```yaml
location: cellar.md?shout=yes#hall
from-page-id: 7f3a91
input:
  say: Hello
```

- `location`: the document's URL, with an optional query, and an optional fragment naming
  a passage.
- `from-page-id`: the `page-id` of the page the request comes from. Absent, the request
  starts the document.
- `input`: optional fields typed in, by name.

The passage shown is the fragment's; without one, the passage of the page the request comes
from; without that, the document's start.

## Responses

A page is a header, then Markdown, which may arrive a piece at a time.

```markdown
---
location: cellar.md#hall
page-id: 8b2c04
title: Hall
journal: true
---

# Hall

You're in a hall. It's dark below.
```

- `location`: where the page is: the request's location, with the passage shown.
- `page-id`: an opaque token identifying the page. A user agent sends it back, as
  `from-page-id`, with requests from the page, and never interprets it. What's behind it is
  the engine's business.
- `title`: the page's title.
- `journal`: the document's hint, if it gives one.

## Following links

A user agent makes the request for a link on a page from how the link is written:

- `#id`: the page's document, with no query, at passage `id`, from the page.
- `?query`: the page's document, with that query and no passage, from the page; so it stays
  on the page's passage.
- Anything else, the document itself included (`[Play again](cellar.md)`): an ordinary
  link. One to a whymark document starts it, with no `from-page-id`.

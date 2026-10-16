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

Given a request and the document's content, the page is determined, apart from what happens
during the page itself: its random numbers, and what capabilities answer, such as an AI.
Those belong to the page: a new page, even for the same request, may differ; the page itself,
made again, doesn't. The request names the page it comes from, which names the one before
it, and so on: a page's **trail**, the pages that led to it, is part of what determines it. So going back shows a
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
  - `resume`: `true` hints that opening the document again should carry on from where its
    reader left it, rather than start it afresh (see [Following links](#following-links)).
    User agents may ignore it.
  - `uses`: the capabilities the document uses (see below), each `optional` (it works
    without it) or `required` (it can't): `uses: {ai: optional}`.

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
- A block named after a capability the document declares, or `without` and its name, is a
  capability block (see [Capabilities](#capabilities)).
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
| `turn.time` | when the page was made, where the engine runs: the fields of Lua's `os.date("*t")` (`year`, `month`, `day`, `hour`, `min`, `sec`, `wday` with Sunday 1, `yday`), and `iso`, the same in ISO 8601 with its UTC offset |
| `turn.notes` | what this page records of itself, an empty table at the start of every turn; plain data only. Unlike `turn.state`, it isn't carried to the next page, but later pages see it (see below) |
| `turn.previous` | the page the request came from, or `nil` on a turn starting the document. It has the `state` it left, its `notes`, its `passage`, its `time`, its request's `query` and `input`, and its own `previous`, back to the start. Changing them changes nothing |
| `turn.write(...)` | writes its arguments to the page |
| `turn.write_headers{title = …}` | writes the page's header (see below) |
| `turn.link(text, query)` | a Markdown link to `?query`, from a table, escaped |
| `turn.go(id)` | stops this passage and shows passage `id` instead; the page's location follows |
| `turn.show(id)` | runs passage `id`, without its heading, where it's called, then carries on |

**Earlier pages:** `turn.previous` and the pages before it are the page's trail, newest
first. A chat, say, keeps each reply in its page's notes, and finds the conversation by
walking back (the reader's side is in each page's `input`):

```lua
local pages, p = {}, turn.previous
while p do
  table.insert(pages, 1, p)   -- oldest first
  p = p.previous
end
```

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

**The sandbox:** the Lua standard libraries base (without `dofile` and `loadfile`), string,
table, math, utf8 and coroutine, and nothing else: no files, apart from the document's
modules (see `require` below), and no network. A turn's CPU and memory are limited.

**Time belongs to the page,** as randomness does: `turn.time` is when the page was made, so a page made again keeps its time, and the sandbox has no clock of its own.

**Randomness belongs to the page:** `math.random` is seeded from the page's own identity, so a
new page, even for the same request, rolls anew, and a page made again rolls the same. A
document wanting dice that follow the story instead keeps a seed in its state, seeds from it
(`math.randomseed`), and draws the next turn's seed into the state:

```lua
function init(turn) return {seed = math.random(1 << 30)} end
```

```lua
math.randomseed(turn.state.seed)
local roll = math.random(6)
turn.state.seed = math.random(1 << 30)
```

**`require`** loads, in order: what it loaded earlier in the turn; a capability the document
declares (see below); or the document's own **Lua module**: `require "lib/story"` runs
`lib/story.lua`, next to the document, in the same sandbox, and returns what it returns.
Module names are paths relative to the document, to a module at or below the document's
directory: `require "../shared"` is an error, and so, for a local document, is a module
reached through a link pointing out of that directory.

### Capabilities

A capability is something a document can't do by itself, which the engine running it may
provide, such as asking an AI. A document declares the capabilities it uses (`uses` in its
front matter); the engine provides those it has, and the reader allows, and a document
reaches one with `require`:

```lua
local ai = require "ai"   -- false if optional and not provided
if ai then … end
```

A capability declared optional, but not provided, gives `false`. One declared required, but
not provided, stops the turn before it runs, with a page saying so. One not declared can't be
required at all: a document's declaration is the complete list of what it can reach.

**Capability blocks:** a fenced block named after a capability the document declares is
handed to the capability. Its lines, code spans run, make a text, which goes to the module's
`__block(turn, args, text)` function, with `args` the words after the name in the info string;
without the capability, the block shows nothing. A block named `without` and the
capability's name is the opposite: ordinary lines, written only when the turn doesn't have
the capability. A block naming a capability the document doesn't declare is just code.

````markdown
```ai
Describe the shop. `if turn.state.oil then` The reader holds a can of oil. `end`
```
```without ai
The pendulums hang still.
```
````

**A failed turn**, whether the document doesn't compile or its code raises an error, gives
a page all the same: the error, and the state the turn started from.

### The AI

The `ai` capability asks an AI. Its module is callable: `ai{…}` asks, and returns the answer;
`ai.write{…}` asks, writes the answer on the page as it arrives, and returns it. Both take a
table:

- `prompt`: the question.
- `messages`: a conversation so far, a list of `{role = "user", content = …}` and
  `{role = "ai", content = …}`, to which `prompt`, if given, adds a turn. It must end with
  the user's turn.
- `system`: instructions, added after the module's `system` field, a document's default
  (`ai.system = "…"`).
- `model`: a tier, `fast`, `balanced` or `smart`, which the engine maps to the models of
  its provider; the module's `model` field is the default.
- `effort`: how hard the AI thinks, `low`, `medium` or `high`, where the provider supports it.
- `returns`, for `ai{…}` only: the shape of the answer, which is then a Lua value instead of
  text: `"string"`, `"number"`, `"integer"` or `"boolean"`; `{shape}` for a list;
  `{field = shape, …}` for a table with those fields; `{one_of = {"a", "b"}}` for one of
  those strings.
- `key`: shares the answer across the document (below).

An `ai` block is `ai.write{prompt = text}`, with the block's text.

The engine's own instructions come first, and include that the AI never writes links: a
document's choices are its own.

**An answer belongs to its page.** It's recorded by the page's id and the call, so every
new page, even for the same request, Reload included, gets new answers, while the pages it
came from keep theirs. A call with a `key` gets the answer recorded for that key and call,
whichever page makes it: an answer that should stay the same across the document. Only
complete answers are recorded; one cut short stays on its page, marked as such.

```lua
local ai = require "ai"
if ai then
  local mood = ai{prompt = "The reader said: " .. turn.input.say, returns = {one_of = {"calm", "angry"}}}
  ai.write{prompt = "The innkeeper, " .. mood .. ", answers the reader."}
end
```

Helpers, such as keeping a story's past answers in its prompts, are Lua modules a document
requires.

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
- `journal`, `resume`: the document's hints, if it gives them.

### Input fields

A page asks the reader for text with an **input block**: a fenced code block whose info
string is `input`, the field's name and, optionally, where to send it (a link written as
`#passage` or `?query`; by default, the page's own passage), and whose content is the
placeholder:

````markdown
```input say
Say something…
```
````

A user agent shows a field instead of the block, while the page is current, and sends what's
typed as a request from the page, to the block's target, with the text as the field's
`input`: `input: {say: …}`. A page has at most one field: if it has several blocks, the last
counts. In other Markdown viewers, the block shows as code.

### Controls

A page's **controls** are what only makes sense while it's current, such as a menu of
actions or "Play again". They're what follows its **controls heading**: a top-level
`## (Controls)` (whatever its case), the last one if there are several. The heading is quiet,
like a quiet passage's: a user agent doesn't show it. A user agent showing earlier pages, as
a trail, may leave their controls out. A page without a controls heading has none.

```markdown
You take the lamp from its hook.

## (Controls)

- [Back to the hall](#hall)
```

## Following links

A user agent makes the request for a link on a page from how the link is written:

- `#id`: the page's document, with no query, at passage `id`, from the page.
- `?query`: the page's document, with that query and no passage, from the page; so it stays
  on the page's passage.
- Anything else, the document itself included (`[Play again](cellar.md)`): an ordinary
  link. One to a whymark document starts it, with no `from-page-id`.

**Resuming:** a user agent may remember the latest page of a document that hints `resume`,
and when the reader opens the document again, from elsewhere and without a query or
passage, carry on from it: a request with that page's `page-id` as `from-page-id`, and no
input. A link to the document from one of its own pages still starts it afresh. For that, an
engine keeps its pages beyond one run of the program; their `page-id`s stay valid as long as
it keeps them. An engine that also keeps what each page's turn wrote lets the user agent show
the page again as it was, with no new turn, so the reader carries on by following its links;
and a user agent showing history, as a journal does, can show the pages that led to it too,
going back through each page's `from-page-id`.

**Page URLs:** a user agent may give each page a URL of its own: its `location`, with its
`page-id` after its passage, separated by `@`, as in `cellar.md#hall@7f3a91`. Opening it
carries on from that page, or shows it again, as resuming does, but from that page rather
than the latest. The
fragment is the user agent's business, never sent to the engine, and a passage's id, a
heading's anchor, never has an `@`. In a plain viewer, it opens the document.

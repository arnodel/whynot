# Writing games for whynot

whynot can show pages that a Lua script writes, instead of a Markdown file. Each time the reader follows a link, the script writes the next page. That's enough for gamebooks, quizzes, puzzles, and anything else made of pages and choices. A script can also ask an AI to write some of the text, or to make decisions.

A game can be written two ways:

- **as a Lua script,** where code writes each page; [lighthouse.lua](lighthouse.lua) is a complete example;
- **as a passage file:** Markdown, where each heading is a page, with a little code where it's needed; [clockmaker.md](clockmaker.md) is a complete example.

Both work the same way underneath, and this guide starts with scripts, which explain how. [Passage files](#passage-files) are near the end.

This guide is for anyone writing a game, person or AI.

## Running a script

```sh
whynot lua:path/to/game.lua
whynot lua:path/to/game.md
```

A path ending in `.md` is a passage file; anything else is a Lua script. The path is relative to the directory whynot runs in, or absolute. The scripts in this guide need nothing else. To let them use an AI, set `ANTHROPIC_API_KEY` (see [Asking an AI](#asking-an-ai)).

When you edit the script, press Reload: every page runs the latest version of the file.

## The smallest game

```lua
function page(request, state)
  state.coins = (state.coins or 0) + (tonumber(request.add) or 0)
  out("# The fountain\n\n")
  out("You have ", state.coins, " coins.\n\n")
  out(link("Throw a coin in", {add = -1}), "\n\n")
  out(link("Fish one out", {add = 1}), "\n")
end
```

A script defines one function, `page`. whynot calls it once for each page, with two tables:

- **`request`** says what the reader chose: the link they followed.
- **`state`** is what the game remembers: the coins here, or the hero's inventory and location.

`page` writes the page with `out`. Everything it writes is Markdown.

## Requests and links

`link(text, request)` returns a Markdown link which, when followed, calls `page` again with `request`:

```lua
out(link("Go north", {go = "north"}))   --> [Go north](?go=north)
```

You can also write such links by hand, as `[Go north](?go=north)`. A link only carries a request if its destination starts with `?`.

In `page`, the request's values are always **strings**: use `tonumber` for numbers. The first page's request is empty, `{}`. Don't use the key `s`: whynot uses it to carry the state. A link that sets `s` to nothing, `[Play again](?s=)`, starts over: see [State](#state).

A link can also lead elsewhere, but then it carries no state: `[Notes](https://example.com)`, or `[Another game](lua:other.lua)`.

## State

On the first page, `state` is empty. If the script defines `init`, whynot calls it first, and what it returns becomes the state:

```lua
function init()
  return {room = "shore", stamina = 3, items = {}}
end
```

whynot calls `init` whenever the state is empty, so also after a restart, or when someone types a URL with a request straight into the address bar. If `init` returns nothing, the state is what it left in `state`.

To start over, link with an empty `s`: `[Play again](?s=)`. Such a link carries no state, so `init` runs again.

Change `state` as you like during `page`. When `page` finishes, whynot keeps the page's state, and every link on the page continues from it. In practice:

- **Back undoes.** Going Back and following another link continues from the earlier page's state, as if the later moves never happened.
- **Reload replays the move.** It calls `page` again with the same request and the same state as before. Dice are rolled again.
- **A page that fails doesn't count.** If the script stops with an error, links on that page continue from the state it started with.

What `state` can hold:

- Only plain data: `nil`, booleans, numbers, strings, and tables of those, nested up to 100 deep. Table keys can be strings, numbers or booleans. A function in `state` is an error.
- In `page`, change the table you're given; don't replace it. `state = {}` only changes your local variable.

**Only `state` lasts.** Each page runs the script from scratch, in a fresh Lua, so global variables set in one page are gone by the next. Use globals for things that never change, such as a table describing the rooms, and `state` for everything that does.

## Writing the page

`out(...)` writes its arguments, strings or numbers, one after the other with nothing between them. `print(...)` also writes into the page, like Lua's own: separated by tabs, with a newline at the end. `page` can also return a string, which is written after everything else.

What you write is Markdown. Some tips:

- **Start with a `#` heading.** It's the page's title, and the window's.
- Put the reader's choices together at the end, for example as a numbered list.
- Leave a blank line, `\n\n`, between paragraphs. A single `\n` doesn't start a new paragraph.
- Everything Markdown does works: emphasis, lists, block quotes, tables, code, and Mermaid diagrams in ` ```mermaid ` blocks.

The page appears as you write it, so a page that takes a while, because of an AI, shows its beginning straight away.

## What Lua offers

Scripts run in a sandbox. They have Lua's basic functions (`pairs`, `ipairs`, `tostring`, `tonumber`, `type`, `pcall`, `error`, `setmetatable`, `select`, `load`…), and the `string`, `table`, `math`, `utf8` and `coroutine` libraries.

They don't have files, the network, `os` (so no time or date), `io`, `require` or `debug`.

For dice, use `math.random`: `math.random(6)` rolls a die. Rolls differ every time, including on Reload.

Each page may use about half a second of CPU and 200 MB of memory. A script that goes over, such as one stuck in a loop, is stopped. Errors and limits show at the bottom of the page, after what was written so far.

## Asking an AI

A script can ask an AI for help, in two ways: to **write** part of a page, and to **decide** something the script then uses. whynot connects the AI: today, Claude, when `ANTHROPIC_API_KEY` is set. Scripts don't name it: they ask for the kind of model they need, and whynot picks one.

### Writing

`ai.write{…}` writes the AI's text into the page at that point, as it arrives, then returns the text:

```lua
ai.system = "You narrate a gothic gamebook, in the second person and the present tense."

function page(request, state)
  out("# The cellar\n\n")
  ai.write{
    prompt  = "Describe the hero entering the cellar.",
    context = {lamp = state.lamp, items = state.items},
  }
  out("\n\n", link("Go back up", {go = "hall"}), "\n")
end
```

The script waits until the text is complete, so the code after it can use it. The fields:

- **`prompt`** (required) is what to write.
- **`context`**, optional, is the game facts the AI should know, as plain data like `state`'s. Pass only what matters. In particular, leave out anything the AI shouldn't give away.
- **`history`**, optional, is a list of earlier texts, the story so far, for the AI to follow on from. To keep one, store the texts in `state`:

  ```lua
  table.insert(state.story, text)
  if #state.story > 3 then table.remove(state.story, 1) end
  ```

  Since it's in `state`, the history follows the reader's path. After Back, the AI only remembers the path the reader is on.
- **`system`**, optional, is extra instructions for this call only, such as a character's voice.
- **`model`** and **`effort`**, optional: see [Models](#models).

`ai.system` is your instructions for every call: the voice, the setting, the facts of the world. whynot's own instructions come first, and yours override them, except one: when writing, the AI never writes links. The choices are yours to make.

### Deciding

`ai{…}` takes the same fields, and returns the AI's answer without showing it. Use it when the script needs the AI's judgment:

```lua
local verdict = ai{
  prompt  = "The hero tries to bribe the guard. Answer only yes or no: does he accept?",
  context = {gold = state.gold, guard = "loyal, but underpaid"},
}
if verdict:lower():find("yes") then state.gate_open = true end
```

For anything more than a word, give the shape of the answer with **`returns`**, and `ai{…}` returns a Lua value of that shape:

```lua
local move = ai{
  prompt  = "Make your next move in twenty questions.",
  context = {answers = state.answers},
  returns = {question = "string", guessing = "boolean", suspects = {"string"}},
}
out("**", move.question, "**")
```

A shape is:

- `"string"`, `"number"`, `"integer"` or `"boolean"`, for a value of that type;
- `{shape}`, a table holding one shape, for a list of them, such as `{"string"}`;
- a table of named shapes, for a table with exactly those fields, all of them present.

The AI's answer is guaranteed to have that shape. [twenty-questions.md](twenty-questions.md) uses it for every move.

### Models

**`model`** says what kind of model a call needs, and `ai.model` sets it for every call:

- `"fast"`: quick and cheap, for a line of atmosphere or a simple judgment;
- `"balanced"`, the default: good writing and reasoning;
- `"smart"`: the most capable, for decisions that need real thought. Slower, and dearer.

whynot maps each to a model of the AI it uses: today, Claude Haiku, Sonnet and Opus. The reader can change the balanced one with `-claude-model`. A model's own name also works, but ties the game to that model.

**`effort`**, `"low"`, `"medium"`, `"high"`, `"xhigh"` or `"max"`, says how hard the model should think, where the model supports it. A `"smart"` model at `"low"` effort is often a good choice for game decisions: capable, and reasonably quick.

### Remembered answers

**Answers are remembered.** Two calls that send exactly the same things get the same answer, without asking the AI again, until whynot quits. "The same things" means all of it: the instructions, `prompt`, `context`, `history`, `model`, `effort` and `returns`. That makes revisits consistent, instant and free, and going Back and choosing the same way again gives the same move. It's also your way to choose when a text changes. Put `state.lamp` in `context`, and the cellar has a lit version and a dark one. Leave it out, and the cellar always reads the same. A `history` makes texts depend on the path, so they're remembered less often.

### Without an AI

Without one, `ai{…}` and `ai.write{…}` write nothing and return `nil`, so a game still runs, minus the AI's text. Lua's `or` gives a fallback: `ai{prompt = "…"} or "no"`. For anything else, `ai.available` is `true` when there's an AI.

## Passage files

A passage file is a game written as Markdown. It reads well as an ordinary document, on GitHub, say, and whynot plays it. Here's a small one:

````markdown
```lua
function init()
  return {tries = 0}
end
```

# The door

An oak door, swollen with salt. You've tried it `= state.tries` times.

- [Force it](#force-it)
- [Walk away](#the-end)

# Force it

`state.tries = state.tries + 1`
`if math.random(6) >= 5 then`
The lock gives way. [Go in](#the-end)
`else`
It doesn't move.

`show("the-door")`
`end`

# The end

That's all. [Play again](?s=)
````

**Passages.** Each level-1 heading starts a passage, which is a page, up to the next level-1 heading. The heading is the page's title. The first passage is where the game starts. Lower headings are just part of a passage.

**Links.** A link to a passage is a link to its heading, as in any Markdown document: `[Force it](#force-it)`. The heading's id is its text in lower case, with spaces turned into hyphens and punctuation dropped; it's the same id that GitHub, or whynot showing the file as a document, would give it. Links to passages carry the state, like the query links of scripts, and other links work as in scripts.

**Code in backquotes runs.** In a passage:

- `` `= expr` `` writes expr's value; `nil` writes nothing.
- Any other single-backquote span is Lua statements: `` `state.tries = state.tries + 1` ``, `` `if has("key") then` ``, `` `else` ``, `` `end` ``.
- A line holding only statements disappears, so control flow around list items or paragraphs leaves no gaps.
- A span with two or more backquotes, ``` `` like this `` ```, is shown as code and doesn't run. So is anything in a code block other than lua and ai ones.
- Code spans don't reach across lines. For several lines of code, use a lua block.

**Lua blocks** (```` ```lua ````) run where they are, and aren't shown. Before the first heading, only lua blocks are allowed: they run before every page, with `state` and `request` set, so that's the place for `init`, helper functions, tables that describe the world, and `ai.system`. A `local` there can be used in every passage.

**`show("id")`** writes another passage where it's called, running its code, without its heading. Use it to end an action with the place it leads to, as "Force it" shows the door again.

**AI blocks** (```` ```ai ````) ask the AI to write their text, the prompt, and show the reply where the block is, as `ai.write{…}` does. Code spans run in the prompt too:

````markdown
```ai
Describe the cellar as the hero walks in.
`if state.lamp then`
The hero carries a lit lamp.
`end`
```
````

Code spans in the prompt run first, and the AI only sees what they write: in that example, the lamp's line is in the prompt only when `state.lamp` is set.

Without an AI, an ai block shows nothing, and its code doesn't run. A **noai block** (```` ```noai ````) is the opposite: ordinary passage text, code spans and all, shown only when there's no AI. Put one after an ai block for a game that reads well either way:

````markdown
```ai
Describe the cellar as the hero walks in.
```
```noai
It smells of tar and paraffin.
```
````

For decisions, or `context`, `history`, `model` and so on, call `ai{…}` or `ai.write{…}` in Lua as in a script, in a lua block or a code span. `ai.write{…}` shows its own text, so call it in a statement, `` `ai.write{…}` ``, not in `` `= …` ``, which would show it twice. [twenty-questions.md](twenty-questions.md) shows a passage file built around `ai{…}` decisions.

**Inside the passage's code**, `state` and `request` are the page's, and `passage` is the current passage's id. Each passage is a Lua function, so a `return` in a statement span, such as `` `if not ok then show("too-late") return end` ``, ends the passage there.

**Errors** give the file's own line numbers, as in `game.md:42`.

**Cmd-U** (Ctrl-U elsewhere) shows the file next to the page it wrote, rendered as Markdown, with its links to passages working.

## Writing a game with an AI

An AI such as Claude can write a whole game: give it this guide, and ask for one. For example:

> Following this guide, write a whynot passage file: a heist in a Venetian palazzo during Carnival. Ten to fifteen passages, an inventory, two ways to win and a few ways to lose, dice for risky actions, and ai blocks for atmosphere.

Save it as a `.md` file (or a `.lua` one, if you asked for a script) and open it with `whynot lua:heist.md`. If something breaks, the error at the bottom of the page is usually enough for the AI to fix it.

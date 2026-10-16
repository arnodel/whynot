---
title: Chat
description: A conversation with an AI, a page for each thing you say.
whymark:
  version: 1
  journal: true
  uses:
    ai: optional
---

A chat with an AI. Each page holds one exchange: what you said, and the answer. The
conversation so far is found again by walking back through the pages that led here (see
[lib/story.lua](lib/story.lua)), so going back and saying something else starts a
different conversation from there. Open it in whynot with an API key for the AI
(`whynot browser/examples/chat.md`; see the viewer's README for the key); [whymark.md](../whymark.md) describes the format.

```lua
local ai = require "ai"
if ai then
  ai.system = [[
You're a friendly, thoughtful conversation partner, chatting in a Markdown viewer. Keep
replies fairly short unless asked for more.
]]
end

local story = require "lib/story"
```

# (Chat)

`local say = turn.input.say`
`if not say or say == "" then`
Say something to start.
`elseif ai then`
> **You:** `= (say:gsub("\n", "\n> "))`

```lua
story.write(turn, ai, {prompt = say})
turn.write("\n")
```
`end`

```without ai
*This chat needs an AI: give whynot an API key, as its README says.*
```

## (Controls)

[Start over](chat.md)

```input say
Say something…
```

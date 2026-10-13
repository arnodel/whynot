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
conversation so far is found again by walking back through the pages that led here, so
going back and saying something else starts a different conversation from there. Open it
in whynot with `ANTHROPIC_API_KEY` set (`whynot browser/examples/chat.md`);
[whymark.md](../whymark.md) describes the format.

```lua
local ai = require "ai"
if ai then
  ai.system = [[
You're a friendly, thoughtful conversation partner, chatting in a Markdown viewer. Keep
replies fairly short unless asked for more.
]]
end

-- conversation returns the exchanges on the pages that led here, oldest
-- first, as the messages of a conversation.
function conversation()
  local messages, p = {}, turn.previous
  while p do
    if p.notes.reply then
      table.insert(messages, 1, {role = "ai", content = p.notes.reply})
      table.insert(messages, 1, {role = "user", content = p.notes.say})
    end
    p = p.previous
  end
  return messages
end
```

# (Chat)

`local say = turn.input.say`
`if not say or say == "" then`
Say something to start.
`elseif ai then`
> **You:** `= (say:gsub("\n", "\n> "))`

```lua
local messages = conversation()
messages[#messages + 1] = {role = "user", content = say}
turn.notes.say = say
turn.notes.reply = ai.write{messages = messages}
turn.write("\n")
```
`end`

```without ai
*This chat needs an AI: start whynot with ``ANTHROPIC_API_KEY`` set.*
```

## (Controls)

[Start over](chat.md)

```input say
Say something…
```

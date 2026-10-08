```lua
-- A chat with the AI, as a passage file. Play it with:
--
--   whynot lua:examples/lua/chat.md
--
-- It needs an AI (ANTHROPIC_API_KEY set). The conversation is kept in
-- state, so going Back and saying something else starts a new branch.

ai.system = [[
You're a friendly, thoughtful conversation partner, chatting in a
Markdown browser. Keep replies fairly short unless asked for more.
]]

function init()
  return {chat = {}}
end
```

# Chat

```lua
if request.say and request.say ~= "" then
  table.insert(state.chat, {role = "user", content = request.say})
end
```
`if #state.chat == 0 then`
Say something to start the conversation.
`end`
`for _, message in ipairs(state.chat) do`
`if message.role == "user" then`

> **You:** `= message.content`

`else`

`= message.content`

`end`
`end`
`if #state.chat > 0 and state.chat[#state.chat].role == "user" then`

```lua
local reply = ai.write{messages = state.chat}
if reply then table.insert(state.chat, {role = "ai", content = reply}) end
```

`end`
`if not ai.available then`
*This chat needs an AI: start whynot with ANTHROPIC_API_KEY set.*
`end`

---

[Say something…](?say={}#end) · [Start over](?s=)

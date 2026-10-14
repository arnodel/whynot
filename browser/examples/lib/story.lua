-- AI answers that remember the pages that led here, with
-- require "lib/story":
--
--   local ai = require "ai"
--   story.write(turn, ai, {prompt = "The reader opens the cabinet."})
--
-- Each exchange, a prompt and its answer, is recorded in the page's
-- turn.notes.story, and the exchanges on the pages that led here, and
-- earlier on this one, are sent before it, as the conversation so far. So the AI sees what it said
-- earlier on this trail, and going back and doing something else forgets
-- what came after.
--
-- story.write and story.ask take what ai.write and ai{...} take; ai is the
-- document's AI module, and without it (false or nil), they do nothing
-- and return nil.

local story = {}

-- history returns the exchanges on the pages that led here, and so far
-- on this one, oldest first, as the messages of a conversation.
function story.history(turn)
  local pages, p = {turn.notes.story}, turn.previous
  while p do
    if p.notes.story then table.insert(pages, 1, p.notes.story) end
    p = p.previous
  end
  local messages = {}
  for _, exchanges in ipairs(pages) do
    for _, e in ipairs(exchanges) do
      messages[#messages + 1] = {role = "user", content = e.prompt}
      messages[#messages + 1] = {role = "ai", content = e.answer}
    end
  end
  return messages
end

-- call asks with ask, ai.write or ai{...}, after the history, and records
-- a text answer to a prompt in turn.notes.story.
local function call(turn, ask, spec)
  local c = {}
  for k, v in pairs(spec) do c[k] = v end
  c.messages = story.history(turn)
  for _, m in ipairs(spec.messages or {}) do c.messages[#c.messages + 1] = m end
  local answer = ask(c)
  if spec.prompt and type(answer) == "string" then
    turn.notes.story = turn.notes.story or {}
    table.insert(turn.notes.story, {prompt = spec.prompt, answer = answer})
  end
  return answer
end

-- write writes the AI's answer on the page, and returns it.
function story.write(turn, ai, spec)
  if not ai then return nil end
  return call(turn, ai.write, spec)
end

-- ask returns the AI's answer.
function story.ask(turn, ai, spec)
  if not ai then return nil end
  return call(turn, ai, spec)
end

return story

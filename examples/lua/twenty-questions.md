```lua
-- Twenty questions: you think of something, and the AI guesses it with
-- yes/no questions. Play it with:
--
--   whynot lua:examples/lua/twenty-questions.md
--
-- It needs an AI (ANTHROPIC_API_KEY set). The rules are Lua; each move,
-- the AI's question and its thinking out loud, comes back from ai{...}
-- as a table.

ai.system = [[
You're playing twenty questions: the reader thinks of something, and you
guess what it is with yes/no questions. Play well: ask questions that
split what's left roughly in half, never ask what's already answered,
and guess when you're fairly sure. Have some personality: curious,
playful, a little competitive.
]]

function init()
  return {answers = {}}
end

-- answer returns a link answering the current question.
function answer(text, value)
  return link(text, {p = "question", answer = value})
end
```

# Twenty questions

Think of something: an animal, an object, a person, a place, anything.
I'll try to guess it in twenty questions or fewer.

`if ai.available then`
- [I've thought of something](#question)
`else`
*This game needs an AI: start whynot with ANTHROPIC_API_KEY set.*
`end`

# Question

```lua
if request.answer and state.question then
  table.insert(state.answers, {question = state.question, answer = request.answer})
  if state.guessing and request.answer == "yes" then show("got-it") return end
end
if #state.answers >= 20 then show("you-win") return end

local move = ai{
  model = "smart", effort = "low",
  prompt = "Make your next move: a yes/no question, or a guess if you're fairly sure.",
  context = {answers = state.answers, questions_left = 20 - #state.answers},
  returns = {
    commentary = "string",
    suspects = {{name = "string", confidence = "integer"}},
    question = "string",
    guessing = "boolean",
  },
}
state.question, state.guessing = move.question, move.guessing
```

*Question `= #state.answers + 1` of 20.*

`= move.commentary`

`if #move.suspects > 0 then`
My suspects:
`for _, suspect in ipairs(move.suspects) do`
- `= suspect.name`, `= suspect.confidence`%
`end`
`end`

**`= move.question`**

`if move.guessing then`
`= answer("Yes, that's it!", "yes")` · `= answer("No", "no")`
`else`
`= answer("Yes", "yes")` · `= answer("No", "no")` · `= answer("Sometimes", "sometimes")` · `= answer("I don't know", "unknown")`
`end`

# Got it

```ai
You've just guessed what the reader was thinking of, after
`= #state.answers` questions: `= state.question`. Gloat a little, in two
sentences.
```
```noai
Got it, in `= #state.answers` questions!
```

[Play again](?s=)

# You win

I give up! Twenty questions, and I still don't know. Well played.

[Play again](?s=)

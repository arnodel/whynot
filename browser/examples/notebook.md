---
title: Notebook
description: A notebook that keeps what you write, page by page.
whymark:
  version: 1
  journal: true
---

A notebook: write something, and it's added to the list. Each entry is the text typed on one
page, found again by walking back through the pages that led here, so going back and writing
something else starts a different notebook from there. Open it in whynot
(`whynot browser/examples/notebook.md`); [whymark.md](../whymark.md) describes the format.

```lua
-- entries returns what was written on the pages that led here, and on
-- this one, oldest first.
function entries()
  local list, p = {}, turn.previous
  while p do
    if p.input.entry then table.insert(list, 1, p.input.entry) end
    p = p.previous
  end
  if turn.input.entry then list[#list + 1] = turn.input.entry end
  return list
end
```

# (Notebook)

`local list = entries()`
`if #list == 0 then`
Nothing written yet.
`else`
`for i, entry in ipairs(list) do`
`= i`. `= entry`
`end`
`end`

```input entry
Write something…
```

---
title: To do
description: A to-do list you talk to. An AI reads what you ask; Lua keeps the list.
whymark:
  version: 1
  resume: true
  uses:
    ai: optional
---

A to-do list you talk to: say what you need ("call the bank by Friday, it's urgent", "milk
and eggs under groceries", "done with the bank"), and the list follows. An AI reads your
request and turns it into a few operations, which Lua checks and applies: the AI never
touches the list itself. Each item's links mark it done or remove it, without asking the AI. Going back undoes, and
journal mode shows the history. The list is still there the next time you open it. Without an
AI, short commands do the same (see the page). Open it in whynot
(`whynot browser/examples/todo.md`, with an API key for the AI, as the viewer's README says);
[whymark.md](../whymark.md) describes the format.

```lua
local ai = require "ai"
local dates = require "lib/dates"

function init(turn)
  return {items = {}, next = 1}
end

-- An item is {id, title, parent (0 for none), due (a day number, or nil),
-- urgent, done}.

local function find(id)
  for _, it in ipairs(turn.state.items) do
    if it.id == id then return it end
  end
end

local function children(parent)
  local list = {}
  for _, it in ipairs(turn.state.items) do
    if it.parent == parent then list[#list + 1] = it end
  end
  return list
end

-- under reports whether item a is b or under it.
local function under(a, b)
  while a do
    if a.id == b.id then return true end
    a = find(a.parent)
  end
  return false
end

-- due parses a due date: a date, or "" for none.
local function due(s)
  if s == nil or s == "" or s == "none" then return nil, true end
  local n = dates.parse(s)
  return n, n ~= nil
end

local ops = {"add", "done", "undone", "remove", "rename", "move", "due", "urgent"}

-- apply applies an operation, {op, id, title, parent, due, urgent}, and
-- returns what it did, or an error. added holds the ids of the items
-- added so far by this request: a parent of -n is the nth of them.
local function apply(op, added)
  local parent = op.parent or 0
  if parent < 0 then parent = added[-parent] or 0 end
  if parent ~= 0 and not find(parent) then return nil, "there's no item " .. parent end
  if op.op == "add" then
    local title = (op.title or ""):match("^%s*(.-)%s*$")
    if title == "" then return nil, "an item needs a title" end
    local d, ok = due(op.due)
    if not ok then return nil, "\"" .. op.due .. "\" isn't a date" end
    local it = {id = turn.state.next, title = title, parent = parent, due = d, urgent = op.urgent == true, done = false}
    turn.state.next = turn.state.next + 1
    table.insert(turn.state.items, it)
    added[#added + 1] = it.id
    return "Added " .. it.title
  end
  local it = find(op.id or 0)
  if not it then return nil, "there's no item " .. tostring(op.id) end
  if op.op == "done" or op.op == "undone" then
    it.done = op.op == "done"
    return (it.done and "Done: " or "Not done: ") .. it.title
  elseif op.op == "remove" then
    local keep = {}
    for _, other in ipairs(turn.state.items) do
      if not under(other, it) then keep[#keep + 1] = other end
    end
    turn.state.items = keep
    return "Removed " .. it.title
  elseif op.op == "rename" then
    if (op.title or "") == "" then return nil, "a new title is needed" end
    local old = it.title
    it.title = op.title
    return "Renamed " .. old .. " to " .. it.title
  elseif op.op == "move" then
    if parent ~= 0 and under(find(parent), it) then return nil, "an item can't go under itself" end
    it.parent = parent
    return "Moved " .. it.title .. (parent == 0 and " to the top" or " under " .. find(parent).title)
  elseif op.op == "due" then
    local d, ok = due(op.due)
    if not ok then return nil, "\"" .. op.due .. "\" isn't a date" end
    it.due = d
    return it.title .. (d and " is due " .. dates.describe(d, dates.today(turn)) or " has no date")
  elseif op.op == "urgent" then
    it.urgent = op.urgent == true
    return it.title .. (it.urgent and " is urgent" or " isn't urgent")
  end
  return nil, "no operation \"" .. tostring(op.op) .. "\""
end

-- describeList writes the list for the AI, an item a line.
local function describeList()
  local lines = {}
  for _, it in ipairs(turn.state.items) do
    local facts = {}
    if it.parent ~= 0 then facts[#facts + 1] = "under " .. it.parent end
    if it.due then facts[#facts + 1] = "due " .. dates.format(it.due) end
    if it.urgent then facts[#facts + 1] = "urgent" end
    if it.done then facts[#facts + 1] = "done" end
    lines[#lines + 1] = it.id .. ": " .. it.title .. (#facts > 0 and " (" .. table.concat(facts, ", ") .. ")" or "")
  end
  return #lines > 0 and table.concat(lines, "\n") or "(empty)"
end

-- recent returns the requests made on the pages before this one, oldest
-- first, at most n of them.
local function recent(n)
  local list, p = {}, turn.previous
  while p and #list < n do
    if p.input.say then table.insert(list, 1, p.input.say) end
    p = p.previous
  end
  return list
end

-- understand asks the AI what a request means, as operations, and a
-- one-line reply.
local function understand(say)
  local today = dates.today(turn)
  local earlier = recent(3)
  local prompt = "Today is " .. dates.weekday(today) .. " " .. dates.format(today) .. ".\n\n" ..
    "The list, an item a line, by id:\n" .. describeList() .. "\n\n" ..
    (#earlier > 0 and "Earlier requests, oldest first:\n- " .. table.concat(earlier, "\n- ") .. "\n\n" or "") ..
    "The request: " .. say
  return ai{
    system = [[
You turn a person's requests about their to-do list into operations on it. Each operation has
every field; the ones it doesn't use are 0, "" or false.
- add: a new item, with its title; parent is the id of the item it goes under, or 0; due and
  urgent as below. To put an item under one added by the same request, use parent -1 for the
  first item added, -2 for the second, and so on.
- done, undone: mark item id done or not done.
- remove: remove item id and everything under it.
- rename: give item id the title.
- move: put item id under parent, or at the top with 0.
- due: give item id the due date, or none with "".
- urgent: make item id urgent or not, with urgent.
Dates are YYYY-MM-DD, worked out from today's date. Refer to items by their ids; when a
request is ambiguous, pick the likeliest reading. The reply is one short, friendly sentence
about what you did, or a question if the request makes no sense for a to-do list (then no
operations).]],
    prompt = prompt,
    model = "balanced",
    returns = {
      ops = {{op = {one_of = ops}, id = "integer", title = "string", parent = "integer", due = "string", urgent = "boolean"}},
      reply = "string",
    },
  }
end

-- parseCommands reads requests written as commands, separated by ";",
-- into operations: add <title> [under <id>] [by <date>] [!], done <id>,
-- undone <id>, remove <id>, rename <id> <title>, move <id> under <id>,
-- move <id> top, due <id> <date|none>, urgent <id>, not urgent <id>.
-- A date is YYYY-MM-DD, today, tomorrow or +N days.
local function parseCommands(say, today)
  local function date(s)
    if s == "today" then return dates.format(today) end
    if s == "tomorrow" then return dates.format(today + 1) end
    local k = s:match("^%+(%d+)$")
    if k then return dates.format(today + tonumber(k)) end
    return s
  end
  local list, bad = {}, {}
  for cmd in (say .. ";"):gmatch("%s*([^;]-)%s*;") do
    if cmd ~= "" then
      local op = {id = 0, title = "", parent = 0, due = "", urgent = false}
      local verb, rest = cmd:match("^(%S+)%s*(.-)$")
      verb = verb:lower()
      if verb == "add" then
        op.op = "add"
        if rest:match("%s*!$") then op.urgent, rest = true, rest:gsub("%s*!$", "") end
        local d = rest:match("%s+by%s+(%S+)$")
        if d then op.due, rest = date(d), rest:gsub("%s+by%s+%S+$", "") end
        local p = rest:match("%s+under%s+(%d+)$")
        if p then op.parent, rest = tonumber(p), rest:gsub("%s+under%s+%d+$", "") end
        op.title = rest
      elseif verb == "not" and rest:match("^urgent%s+%d+$") then
        op.op, op.id = "urgent", tonumber(rest:match("%d+"))
      elseif verb == "rename" then
        local id, title = rest:match("^(%d+)%s+(.+)$")
        op.op, op.id, op.title = "rename", tonumber(id), title
      elseif verb == "move" then
        local id, p = rest:match("^(%d+)%s+under%s+(%d+)$")
        if id then
          op.op, op.id, op.parent = "move", tonumber(id), tonumber(p)
        else
          op.op, op.id = "move", tonumber(rest:match("^(%d+)%s+top$"))
        end
      elseif verb == "due" then
        local id, d = rest:match("^(%d+)%s+(%S+)$")
        op.op, op.id, op.due = "due", tonumber(id), d and date(d) or nil
      elseif verb == "urgent" then
        op.op, op.id, op.urgent = "urgent", tonumber(rest:match("^%d+$")), true
      else
        op.op, op.id = verb, tonumber(rest:match("^%d+$"))
      end
      if op.op and op.id ~= nil and op.due ~= nil then
        list[#list + 1] = op
      else
        bad[#bad + 1] = cmd
      end
    end
  end
  return list, bad
end

-- act carries out the action of an item's link (done, undone or
-- remove), and returns what it did, or an error.
function act(action, id)
  if action ~= "done" and action ~= "undone" and action ~= "remove" then
    return nil, "no action \"" .. tostring(action) .. "\""
  end
  return apply({op = action, id = tonumber(id) or 0}, {})
end

-- handle carries out a request, and returns the reply to show, what
-- changed, and what couldn't be done.
function handle(say)
  local reply, list, problems = nil, {}, {}
  if ai then
    local r = understand(say)
    reply, list = r.reply, r.ops
  else
    local bad
    list, bad = parseCommands(say, dates.today(turn))
    for _, cmd in ipairs(bad) do problems[#problems + 1] = "didn't understand \"" .. cmd .. "\"" end
  end
  local changes, added = {}, {}
  for _, op in ipairs(list) do
    local done, err = apply(op, added)
    if done then changes[#changes + 1] = done else problems[#problems + 1] = err end
  end
  return reply, changes, problems
end

-- escape keeps text from being read as Markdown.
local function escape(s)
  return (tostring(s):gsub("([\\`*_%[%]<>#|])", "\\%1"))
end

-- latest returns the earliest due date and whether anything's urgent,
-- among item it and the undone items under it, for sorting.
local function summary(it)
  local d, urgent = not it.done and it.due or nil, not it.done and it.urgent
  for _, c in ipairs(children(it.id)) do
    local cd, cu = summary(c)
    if cd and (not d or cd < d) then d = cd end
    urgent = urgent or cu
  end
  return d, urgent
end

local function sorted(list)
  table.sort(list, function(a, b)
    local da, ua = summary(a)
    local db, ub = summary(b)
    if a.done ~= b.done then return not a.done end
    if ua ~= ub then return ua end
    if da ~= db then
      if not da or not db then return da ~= nil end
      return da < db
    end
    return a.id < b.id
  end)
  return list
end

-- writeItem writes an item and those under it, as task list items.
local function writeItem(it, depth, today)
  local facts = {}
  if it.due and not it.done then
    local k = it.due - today
    facts[#facts + 1] = (k < 0 and "**overdue**, " or "") .. "due " .. dates.describe(it.due, today)
  end
  if it.urgent and not it.done then facts[#facts + 1] = "**urgent**" end
  local toggle = it.done and turn.link("undo", {act = "undone", id = it.id}) or turn.link("done", {act = "done", id = it.id})
  turn.write(string.rep("  ", depth), "- [", it.done and "x" or " ", "] ", escape(it.title),
    #facts > 0 and " · " .. table.concat(facts, " · ") or "", " · *#", it.id, "* · ",
    toggle, " · ", turn.link("remove", {act = "remove", id = it.id}), "\n")
  for _, c in ipairs(sorted(children(it.id))) do
    writeItem(c, depth + 1, today)
  end
end

-- show writes the list, in sections by when things are due.
function show()
  local today = dates.today(turn)
  local sections = {
    {"Overdue", function(d) return d and d < today end},
    {"Today", function(d) return d == today end},
    {"This week", function(d) return d and d > today and d < today + 7 end},
    {"Later", function(d) return d and d >= today + 7 end},
    {"Someday", function(d) return d == nil end},
  }
  local top, done = {}, {}
  for _, it in ipairs(children(0)) do
    if it.done then done[#done + 1] = it else top[#top + 1] = it end
  end
  if #top == 0 and #done == 0 then
    turn.write("Nothing to do yet.\n\n")
    return
  end
  for _, s in ipairs(sections) do
    local items = {}
    for _, it in ipairs(top) do
      if s[2]((summary(it))) then items[#items + 1] = it end
    end
    if #items > 0 then
      turn.write("## ", s[1], "\n\n")
      for _, it in ipairs(sorted(items)) do writeItem(it, 0, today) end
      turn.write("\n")
    end
  end
  if #done > 0 then
    turn.write("## Done\n\n")
    for _, it in ipairs(sorted(done)) do writeItem(it, 0, today) end
    turn.write("\n")
  end
end
```

# (To do)

```lua
if turn.query.act then
  local done, err = act(turn.query.act, turn.query.id)
  turn.write("- ", done or "Couldn't: " .. err, "\n\n")
end
local say = turn.input.say
if say and say ~= "" then
  local reply, changes, problems = handle(say)
  turn.write("> ", (say:gsub("\n", " ")), "\n\n")
  if reply and reply ~= "" then turn.write("*", (reply:gsub("\n", " ")), "*\n\n") end
  for _, c in ipairs(changes) do turn.write("- ", c, "\n") end
  for _, p in ipairs(problems) do turn.write("- Couldn't: ", p, "\n") end
  if #changes + #problems > 0 then turn.write("\n") end
end
show()
```

```without ai
*Without an AI, use commands, separated by* ``;``*:* ``add Call the bank by tomorrow !``*,*
``add Milk under 2``*,* ``done 3``*,* ``due 1 +3``*,* ``move 4 under 2``*,* ``remove 5``*. A
date is YYYY-MM-DD,* ``today``*,* ``tomorrow`` *or* ``+N`` *days;* ``!`` *makes it urgent.*
```

## (Controls)

[Start over](todo.md)

```input say
What needs doing?
```

---
title: The Clockmaker's Shop
description: A tiny game against the clock, written as passages.
whymark:
  version: 1
---

The Clockmaker's Shop: a tiny game written as a whymark document. Open it in whynot to play
(`whynot browser/examples/clockmaker.md`). Read as Markdown, it's also the game's script:
each `#` heading is a page, and the code in backquotes runs. See
[whymark.md](../whymark.md) for the format.

```lua
function init(turn)
  return {minutes = 10, items = {}}
end

function has(item)
  for _, it in ipairs(turn.state.items) do
    if it == item then return true end
  end
  return false
end

-- spend takes minutes off the clock, and reports whether any are left.
function spend(minutes)
  turn.state.minutes = turn.state.minutes - minutes
  return turn.state.minutes > 0
end

-- footer writes the page's status, after its menu: what follows the
-- page's last "---", its controls, which journal mode leaves out of the
-- earlier pages.
function footer()
  local carrying = #turn.state.items > 0 and table.concat(turn.state.items, ", ") or "nothing"
  turn.write("\n*", turn.state.minutes, " minutes to midnight · carrying ", carrying, "*\n")
end
```

# The shop

Every clock in the shop has stopped, at ten to midnight. Your master is
nowhere to be found, and the great clock upstairs must be wound before
midnight strikes.

```ai
Describe the shop as the reader stands in it, among the stopped clocks.
`if has("oil") then`
They hold a can of clock oil.
`end`
```
```noai
The pendulums hang still. Somewhere, a spring creaks as it cools.
```

---

- [Search the workbench](#the-workbench)
- [Open the cabinet](#the-cabinet)
- [Climb the stairs](#the-stairs)

`footer()`

# The workbench

`if not spend(1) then turn.go("too-late") end`
`if has("key") then`
Tools, springs and a cold cup of tea. Nothing else of use.
`else`
`local roll = math.random(6)`
You rummage through drawers of springs and screws. You roll a `= roll`.
`if roll >= 3 then`
`table.insert(turn.state.items, "key")`
Under a tray of hands: a small brass key.
`else`
Nothing but a pricked finger.
`end`
`end`

---

- [Back to the shop](#the-shop)

`footer()`

# The cabinet

`if not spend(1) then turn.go("too-late") end`
`if has("oil") then`
The cabinet stands open and empty.
`elseif has("key") then`
`table.insert(turn.state.items, "oil")`
The brass key turns. Inside, on a velvet shelf: a can of clock oil.
`else`
The cabinet is locked. The keyhole is small, and brass.
`end`

---

- [Back to the shop](#the-shop)

`footer()`

# The stairs

`if not spend(2) then turn.go("too-late") end`
The great clock fills the attic: wheels as tall as you, all seized.
`if has("oil") then`

---

- [Oil the wheels and wind the clock](#midnight)
`else`
Its gears won't move. They need oil.

---

- [Go back down](#the-shop)
`end`

`footer()`

# Midnight

The wheels turn, the pendulum swings, and the great clock strikes twelve.
Downstairs, every clock in the shop starts ticking again.

**You've won**, with `= turn.state.minutes` minutes to spare. [Play again](clockmaker.md)

# Too late

Somewhere below, a clock begins to strike. It's midnight, and the great
clock is still silent.

**The End.** [Try again](clockmaker.md)

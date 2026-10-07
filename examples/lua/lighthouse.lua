-- The Lighthouse: a tiny gamebook written for whynot's lua: pages.
--
--   whynot lua:examples/lua/lighthouse.lua
--
-- The rules and the dice are Lua. With ANTHROPIC_API_KEY set, Claude adds
-- a paragraph of atmosphere to each scene, written once per situation and
-- then remembered.

claude.system = [[
You narrate a short gothic gamebook, in the second person and the present
tense. The hero has rowed out to a lighthouse on a rock off the Cornish
coast, on a stormy night, to light its beacon before a ship strikes the
reef. Write one paragraph of three or four sentences: atmosphere and
sensations only. Don't make the hero act, and don't add items or exits.
]]

local scenes = {
  shore = {
    title = "The landing",
    text = "You drag the boat up the slipway. Above you, the lighthouse door is black and shut.",
    exits = {{"Climb to the door", "door"}},
  },
  door = {
    title = "The door",
    text = "An oak door, swollen with salt, and a heavy iron lock.",
  },
  rocks = {
    title = "The rocks",
    text = "Rock pools, weed, and the wreck of an old crab pot wedged in a crevice.",
    exits = {{"Go back to the door", "door"}},
  },
  hall = {
    title = "The hall",
    text = "A round stone room. Stairs spiral up into darkness; a trapdoor leads down.",
    exits = {{"Climb the stairs", "stairs"}, {"Go down to the cellar", "cellar"}},
  },
  cellar = {
    title = "The cellar",
    text = "It smells of tar and paraffin. Barrels line the walls.",
    exits = {{"Go back up", "hall"}},
  },
  lantern = {
    title = "The lantern room",
    text = "Glass on every side, rattling in the wind, and the great lamp in the middle. Out in the dark, a ship's lights are closing on the reef.",
  },
}

local function has(state, item)
  for _, it in ipairs(state.items) do
    if it == item then return true end
  end
  return false
end

local function roll() return math.random(6) end

-- act applies the action in request to state, and returns what happened.
local function act(request, state)
  if request.go then
    if request.go == "door" and has(state, "key") then
      state.scene = "hall"
      return "The key turns with a groan. You're in."
    elseif request.go == "stairs" then
      if not has(state, "lamp") then
        state.stamina = state.stamina - 1
        return "You climb blind, miss a step and crack your shin. Better find a light."
      end
      state.scene = "lantern"
      return "You climb a hundred and twelve steps, lamp in hand."
    end
    state.scene = request.go
  elseif request.force then
    local d = roll()
    if d >= 5 then
      state.scene = "hall"
      return "You roll a " .. d .. ". The lock tears out of the rotten wood and you stumble in."
    end
    state.stamina = state.stamina - 1
    return "You roll a " .. d .. ". The door doesn't move, but your shoulder does."
  elseif request.search then
    local d = roll()
    if d >= 3 then
      state.searched = true
      table.insert(state.items, "key")
      return "You roll a " .. d .. ". Under the crab pot, wrapped in oilcloth: a key."
    end
    state.stamina = state.stamina - 1
    return "You roll a " .. d .. ". You slip on the weed and fall hard."
  elseif request.take then
    table.insert(state.items, request.take)
    return "You take the " .. request.take .. "."
  elseif request.light then
    state.won = true
    return "You fill the reservoir and strike a match."
  end
end

-- choices returns the links for the hero's current situation.
local function choices(state)
  local scene, list = state.scene, {}
  for _, exit in ipairs(scenes[scene].exits or {}) do
    list[#list + 1] = link(exit[1], {go = exit[2]})
  end
  if scene == "door" and not has(state, "key") then
    list[#list + 1] = link("Force the door", {force = 1})
    list[#list + 1] = link("Search the rocks", {go = "rocks"})
  elseif scene == "door" then
    list[#list + 1] = link("Unlock the door", {go = "door"})
  elseif scene == "rocks" and not state.searched then
    list[#list + 1] = link("Search the crab pot", {search = 1})
  elseif scene == "hall" and not has(state, "lamp") then
    list[#list + 1] = link("Take the lamp from its hook", {take = "lamp"})
  elseif scene == "cellar" and not has(state, "paraffin") then
    list[#list + 1] = link("Take a can of paraffin", {take = "paraffin"})
  elseif scene == "lantern" and has(state, "paraffin") then
    list[#list + 1] = link("Light the beacon", {light = 1})
  elseif scene == "lantern" then
    list[#list + 1] = link("Go back down", {go = "hall"})
  end
  return list
end

function page(request, state)
  if not state.scene then
    state.scene, state.stamina, state.items, state.story = "shore", 3, {}, {}
  end
  local happened = act(request, state)
  local scene = scenes[state.scene]

  out("# ", scene.title, "\n\n")
  if happened then out("*", happened, "*\n\n") end

  if state.stamina <= 0 then
    out("You sink down against the cold stone, too hurt to go on. Out on the reef, a ship's timbers split.\n\n",
        "**The End.** ", "[Try again](lua:examples/lua/lighthouse.lua)\n")
    return
  end
  if state.won then
    out("The beacon blazes out over the water. The ship's lights turn, slowly, away from the reef.\n\n",
        "**You've won.** ", "[Play again](lua:examples/lua/lighthouse.lua)\n")
    return
  end

  out(scene.text, "\n\n")
  if claude.available then
    local text = claude{
      prompt = "Describe this moment, in the scene: " .. scene.title .. ". " .. scene.text,
      context = {items = state.items, stamina = state.stamina, lamp_lit = has(state, "lamp")},
      history = state.story,
    }
    table.insert(state.story, text)
    if #state.story > 3 then table.remove(state.story, 1) end
    out("\n\n")
  end

  out("## What do you do?\n\n")
  for i, choice in ipairs(choices(state)) do
    out(i, ". ", choice, "\n")
  end
  local carrying = #state.items > 0 and table.concat(state.items, ", ") or "nothing"
  out("\n---\n\n*Stamina ", state.stamina, " · carrying ", carrying, "*\n")
end

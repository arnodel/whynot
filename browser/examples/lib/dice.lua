-- Dice for whymark documents, with require "lib/dice":
--
--   dice.roll("2d6")              -- a free roll
--   dice.fixed_roll(turn, "2d6")  -- a roll that follows the story
--
-- A free roll belongs to the page: a new page, Reload included, rolls
-- anew. A fixed roll follows the story: the same trail rolls the same, so
-- going back and trying again only helps if something else changed. Its
-- seed is kept in turn.state.dice; call dice.init(state) in init(turn) so
-- that even the first fixed roll follows the story.
--
-- A roll is a number of sides, or dice notation: "d20", "3d6", "2d6+1".

local dice = {}

-- parse returns a roll's number of dice, sides and modifier.
local function parse(spec)
  if math.type(spec) == "integer" then
    if spec < 1 then error("a die needs at least one side", 3) end
    return 1, spec, 0
  end
  local n, sides, sign, k
  if type(spec) == "string" then
    n, sides, sign, k = spec:match("^%s*(%d*)[dD](%d+)%s*([-+]?)%s*(%d*)%s*$")
  end
  if not sides or (sign == "") ~= (k == "") then
    error("not a roll: " .. tostring(spec) .. " (want sides, or dice such as 2d6+1)", 3)
  end
  n, sides, k = tonumber(n) or 1, tonumber(sides), tonumber(k) or 0
  if n < 1 or n > 1000 or sides < 1 then
    error("not a roll: " .. spec, 3)
  end
  return n, sides, sign == "-" and -k or k
end

local function total(n, sides, k)
  for _ = 1, n do k = k + math.random(sides) end
  return k
end

-- roll rolls freely, from the page's own randomness.
function dice.roll(spec)
  return total(parse(spec))
end

-- init gives state a seed for fixed rolls, if it has none, and returns it.
function dice.init(state)
  state.dice = state.dice or math.random(1 << 62)
  return state
end

-- fixed_roll rolls from the seed in turn.state.dice, and draws the next
-- seed into it. Free rolls after it on the page stay free.
function dice.fixed_roll(turn, spec)
  local n, sides, k = parse(spec)
  dice.init(turn.state)
  local resume = math.random(1 << 62)
  math.randomseed(turn.state.dice)
  local result = total(n, sides, k)
  turn.state.dice = math.random(1 << 62)
  math.randomseed(resume)
  return result
end

return dice

-- Dates as day numbers, days since 1970-01-01, for whymark documents,
-- with require "lib/dates":
--
--   local today = dates.today(turn)          -- from turn.time
--   local due = dates.parse("2026-10-20")    -- nil if it isn't a date
--   dates.describe(due, today)               -- "in 4 days", "tomorrow", ...
--
-- Day numbers subtract and compare as plain integers.

local dates = {}

local weekdays = {"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

-- day returns the day number of a date (proleptic Gregorian).
function dates.day(y, m, d)
  if m <= 2 then y = y - 1 end
  local era = (y >= 0 and y or y - 399) // 400
  local yoe = y - era * 400
  local doy = (153 * ((m + 9) % 12) + 2) // 5 + d - 1
  local doe = yoe * 365 + yoe // 4 - yoe // 100 + doy
  return era * 146097 + doe - 719468
end

-- civil returns the year, month and day of a day number.
function dates.civil(n)
  n = n + 719468
  local era = (n >= 0 and n or n - 146096) // 146097
  local doe = n - era * 146097
  local yoe = (doe - doe // 1460 + doe // 36524 - doe // 146096) // 365
  local doy = doe - (365 * yoe + yoe // 4 - yoe // 100)
  local mp = (5 * doy + 2) // 153
  local d = doy - (153 * mp + 2) // 5 + 1
  local m = mp < 10 and mp + 3 or mp - 9
  local y = yoe + era * 400
  if m <= 2 then y = y + 1 end
  return y, m, d
end

-- parse returns the day number of a date written YYYY-MM-DD, or nil.
function dates.parse(s)
  local y, m, d = tostring(s):match("^%s*(%d%d%d%d)-(%d%d)-(%d%d)%s*$")
  if not y then return nil end
  y, m, d = tonumber(y), tonumber(m), tonumber(d)
  local n = dates.day(y, m, d)
  local y2, m2, d2 = dates.civil(n)
  if y2 ~= y or m2 ~= m or d2 ~= d then return nil end -- such as 2026-02-30
  return n
end

-- format writes a day number as YYYY-MM-DD.
function dates.format(n)
  return string.format("%04d-%02d-%02d", dates.civil(n))
end

-- today returns the day number of the day the page was made.
function dates.today(turn)
  local t = turn.time
  return dates.day(t.year, t.month, t.day)
end

-- weekday returns the name of a day number's day of the week.
function dates.weekday(n)
  return weekdays[(n + 4) % 7 + 1]
end

-- describe says when n is, from today: "today", "tomorrow", "on
-- Friday" within the week, "in 12 days", or "2 days ago".
function dates.describe(n, today)
  local k = n - today
  if k == 0 then return "today"
  elseif k == 1 then return "tomorrow"
  elseif k == -1 then return "yesterday"
  elseif k > 1 and k < 7 then return "on " .. dates.weekday(n)
  elseif k >= 7 then return "in " .. k .. " days"
  end
  return -k .. " days ago"
end

return dates

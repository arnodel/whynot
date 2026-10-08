package browser

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const doorPassages = "```lua" + `
function init()
  return {items = {}, tries = 0}
end

local function has(item)
  for _, it in ipairs(state.items) do
    if it == item then return true end
  end
end
` + "```" + `

# The *door*

An oak door. Tries: ` + "`= state.tries`" + `.
` + "`if has(\"key\") then`" + `
- [Unlock it](#the-hall)
` + "`else`" + `
- [Force it](#force-it)
- [Find the key](#the-rocks)
` + "`end`" + `

# Force it

` + "```lua" + `
state.tries = state.tries + 1
` + "```" + `

` + "`show(\"the-door\")`" + `

# The rocks

` + "`table.insert(state.items, \"key\")`" + `
You find a key. Write ` + "``code``" + ` to show code.

- [Back](#the-door)

# The hall

You're in. ` + "`= nil`" + `[Play again](?s=)
`

// TestPassages plays a passage file: the first passage, a passage that
// shows another, statements that change state, and a restart.
func TestPassages(t *testing.T) {
	res := &LuaResolver{}
	start := writeScriptFile(t, "game.md", doorPassages)

	u, page := loadPage(t, res, start, "")
	want := "# The *door*\n\nAn oak door. Tries: 0.\n- [Force it](?s=1&p=force-it)\n- [Find the key](?s=1&p=the-rocks)\n\n"
	if page != want {
		t.Fatalf("first page = %q, want %q", page, want)
	}

	u, page = loadPage(t, res, u, linkDest(t, page, "Force it"))
	if !strings.HasPrefix(page, "# Force it\n") || !strings.Contains(page, "Tries: 1.") {
		t.Fatalf("Force it = %q, want its heading, then the door with one try", page)
	}

	u, page = loadPage(t, res, u, linkDest(t, page, "Find the key"))
	if !strings.Contains(page, "Write ``code`` to show code.") {
		t.Errorf("The rocks = %q, want the double-backquoted code shown", page)
	}
	u, page = loadPage(t, res, u, linkDest(t, page, "Back"))
	if !strings.Contains(page, "[Unlock it]") || strings.Contains(page, "Force it") {
		t.Fatalf("the door with the key = %q, want only the Unlock link", page)
	}

	u, page = loadPage(t, res, u, linkDest(t, page, "Unlock it"))
	if want := "[Play again](?s=)"; !strings.Contains(page, want) {
		t.Fatalf("The hall = %q, want %q: a link with an s keeps it", page, want)
	}
	_, page = loadPage(t, res, u, linkDest(t, page, "Play again"))
	if !strings.Contains(page, "Tries: 0.") || !strings.Contains(page, "Find the key") {
		t.Errorf("after Play again = %q, want the door from init's state", page)
	}
}

func TestPassageErrors(t *testing.T) {
	for _, c := range []struct{ name, source, want string }{
		{"prose before the first passage", "Hello\n\n# Start\n", "line 1: before the first passage"},
		{"no passages", "```lua\nx = 1\n```\n", "no passages"},
		{"runtime error line", "# Start\n\nfine\n`error(\"boom\")`\n", "game.md:4"},
		{"syntax error line", "# Start\n\n`if x then`\n\n# Next\n", "game.md:5"},
		{"unknown passage", "# Start\n\n`show(\"nowhere\")`\n", `no passage "nowhere"`},
		{"unclosed lua block", "# Start\n\n```lua\nx = 1\n", "line 3: a lua block needs its closing fence"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, page := loadPage(t, &LuaResolver{}, writeScriptFile(t, "game.md", c.source), "")
			if !strings.Contains(page, c.want) {
				t.Errorf("page = %q, want it to mention %q", page, c.want)
			}
		})
	}
}

func TestCompileLine(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"plain", `out("plain") out("\n") `},
		{"`if x then`", "if x then "},
		{"  `end`  `end` ", "end end "},
		{"a `= x` b", `out("a ") __value(x) out(" b") out("\n") `},
		{"`= x`", `__value(x) out("\n") `},
		{"``literal`` and \\`not code\\`", "out(\"``literal`` and \\\\`not code\\\\`\") out(\"\\n\") "},
		{"unclosed ` tick", "out(\"unclosed ` tick\") out(\"\\n\") "},
	} {
		got, err := compileLine(c.line, false)
		if err != nil || got != c.want {
			t.Errorf("compileLine(%q) = %q, %v; want %q", c.line, got, err, c.want)
		}
	}
}

// TestPassageAI checks that an ai block sends its text as the prompt,
// with its spans run, and shows the reply where it is.
func TestPassageAI(t *testing.T) {
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []AIMessage }
		json.NewDecoder(r.Body).Decode(&body)
		prompts = append(prompts, body.Messages[0].Content)
		io.WriteString(w, `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"It is dark."}}`+"\n\n")
	}))
	defer server.Close()

	script := writeScriptFile(t, "game.md", aiCellar)
	_, page := loadPage(t, &LuaResolver{AI: &Anthropic{Endpoint: server.URL}}, script, "")
	if want := "Describe the cellar.\nStamina: 3.\n"; len(prompts) != 1 || prompts[0] != want {
		t.Errorf("prompts = %q, want [%q]", prompts, want)
	}
	if want := "You go down.\n\nIt is dark.\n\n- [Back up]"; !strings.Contains(page, want) || strings.Contains(page, "Without") {
		t.Errorf("page = %q, want it to contain %q, and not the noai block", page, want)
	}
}

const aiCellar = "# The cellar\n\nYou go down.\n\n```ai\nDescribe the cellar.\n`if state.lamp then`\nThe hero has a lamp.\n`end`\nStamina: `= 3`.\n```\n```noai\nWithout an AI, `= 2 + 2`.\n```\n\n- [Back up](#the-cellar)\n"

// TestPassageWithoutAI checks that without an AI, an ai block shows
// nothing, a noai block shows, and ai{} returns nil.
func TestPassageWithoutAI(t *testing.T) {
	_, page := loadPage(t, &LuaResolver{}, writeScriptFile(t, "game.md", aiCellar), "")
	if want := "You go down.\n\nWithout an AI, 4.\n\n- [Back up]"; !strings.Contains(page, want) {
		t.Errorf("page = %q, want it to contain %q", page, want)
	}
	_, page = loadPage(t, &LuaResolver{}, writeScript(t, "function page() out(ai{prompt = 'Hi'} or 'fallback') end"), "")
	if page != "fallback" {
		t.Errorf("page = %q, want %q: ai{} returns nil without an AI", page, "fallback")
	}
}

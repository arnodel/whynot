package browser

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
)

// A Key is a key press that runs a command: the key's name, and the
// modifiers held with it. Names are a key's main symbol: a letter or
// digit in upper case ("J", "0"), a punctuation mark ("[", "=", "/"), or
// one of "Space", "Backspace", "Escape", "Up", "Down", "Left" and
// "Right".
type Key struct {
	Name string
	// Shortcut is the platform's modifier for shortcuts: Cmd on macOS,
	// Ctrl elsewhere.
	Shortcut, Shift, Alt bool
	// Repeats is whether the key runs its command again while held, as
	// for scrolling and zooming.
	Repeats bool
}

// command is something the browser does on request: from a key, or from
// a link on the actions page (see ShowActions).
type command struct {
	// name is the command's name in the actions page's links:
	// "whynot:name".
	name string
	keys []Key
	run  func(a *App)
	// closes is whether the actions page closes before a link to it runs
	// the command: it changes what's shown, rather than showing on the
	// actions page itself, as zoom and the theme do.
	closes bool
}

// macOS is whether shortcuts follow macOS's conventions.
var macOS = runtime.GOOS == "darwin"

// commands are the browser's commands. They're set by init, since some
// of them refer back to the list, through the actions page.
var commands []command

func init() {
	commands = []command{
		{name: "back", keys: backKeys(), run: (*App).Back, closes: true},
		{name: "forward", keys: forwardKeys(), run: (*App).Forward, closes: true},
		{name: "reload", keys: []Key{{Name: "R", Shortcut: true}}, run: (*App).Reload, closes: true},
		{name: "toc", keys: []Key{{Name: "T", Shortcut: true}}, run: (*App).ToggleTOC, closes: true},
		{name: "journal", keys: []Key{{Name: "J", Shortcut: true}}, run: (*App).ToggleJournal, closes: true},
		{name: "theme", keys: []Key{{Name: "L", Shortcut: true, Shift: true}}, run: func(a *App) { a.SetTheme(!a.darkTheme) }},
		{name: "zoom-in", keys: []Key{{Name: "=", Shortcut: true, Repeats: true}}, run: (*App).ZoomIn},
		{name: "zoom-out", keys: []Key{{Name: "-", Shortcut: true, Repeats: true}}, run: (*App).ZoomOut},
		{name: "zoom-reset", keys: []Key{{Name: "0", Shortcut: true}}, run: func(a *App) { a.SetZoom(1) }},
		{name: "paste", keys: []Key{{Name: "V", Shortcut: true}}, run: (*App).Paste, closes: true},
		{name: "copy", keys: []Key{{Name: "C", Shortcut: true, Shift: true}}, run: (*App).CopyMarkdown},
		{name: "actions", keys: []Key{{Name: "/", Shortcut: true}, {Name: "/", Shift: true}}, run: (*App).ToggleActions},
		{name: "close", keys: []Key{{Name: "Escape"}}, run: (*App).HideOverlay},
		{name: "scroll-down", keys: []Key{{Name: "Down", Repeats: true}}, run: (*App).ScrollDown},
		{name: "scroll-up", keys: []Key{{Name: "Up", Repeats: true}}, run: (*App).ScrollUp},
		{name: "page-down", keys: []Key{{Name: "Space"}}, run: (*App).PageDown},
		{name: "page-up", keys: []Key{{Name: "Space", Shift: true}}, run: (*App).PageUp},
		{name: "scroll-left", keys: []Key{{Name: "Left", Repeats: true}}, run: (*App).ScrollLeft},
		{name: "scroll-right", keys: []Key{{Name: "Right", Repeats: true}}, run: (*App).ScrollRight},
	}
}

// backKeys and forwardKeys follow the platform's browsers: Cmd+[ and
// Cmd+Left on macOS, Alt+Left elsewhere; Backspace too, as browsers used
// to.
func backKeys() []Key {
	if macOS {
		return []Key{{Name: "[", Shortcut: true}, {Name: "Left", Shortcut: true}, {Name: "Backspace"}}
	}
	return []Key{{Name: "Left", Alt: true}, {Name: "Backspace"}}
}

func forwardKeys() []Key {
	if macOS {
		return []Key{{Name: "]", Shortcut: true}, {Name: "Right", Shortcut: true}, {Name: "Backspace", Shift: true}}
	}
	return []Key{{Name: "Right", Alt: true}, {Name: "Backspace", Shift: true}}
}

// Keys returns the keys that run a command, for a program to watch for
// and pass to PressKey.
func (a *App) Keys() []Key {
	var keys []Key
	for _, c := range commands {
		keys = append(keys, c.keys...)
	}
	return keys
}

// PressKey runs the command k runs, if any, reporting whether there was
// one. A key's command runs as it is, whatever is showing: Back closes
// an overlay, for instance, as it does from the toolbar.
func (a *App) PressKey(k Key) bool {
	a.touch = false
	k.Repeats = false
	for _, c := range commands {
		if slices.ContainsFunc(c.keys, func(ck Key) bool { ck.Repeats = false; return ck == k }) {
			c.run(a)
			return true
		}
	}
	return false
}

// runCommand runs the command name, from a link on the actions page:
// closing the page first if the command changes what's shown, or else
// showing the page again, as the command left things.
func (a *App) runCommand(name string) {
	i := slices.IndexFunc(commands, func(c command) bool { return c.name == name })
	if i < 0 {
		return
	}
	c := commands[i]
	if c.closes {
		a.hideOverlay()
	}
	c.run(a)
	if a.overlay == actionsOverlay {
		a.refreshActions()
	}
}

// keyNames is how keys whose name isn't the symbol on them are written.
var keyNames = map[string]string{
	"Up": "↑", "Down": "↓", "Left": "←", "Right": "→",
	"Escape": "Esc", "-": "−",
}

// String returns k as a person would write it: "Cmd+J", "Shift+Space".
func (k Key) String() string {
	var b strings.Builder
	if k.Shortcut {
		if macOS {
			b.WriteString("Cmd+")
		} else {
			b.WriteString("Ctrl+")
		}
	}
	if k.Alt {
		b.WriteString("Alt+")
	}
	name := k.Name
	switch {
	case name == "/" && k.Shift && !k.Shortcut:
		return b.String() + "?"
	case k.Shift:
		b.WriteString("Shift+")
	}
	if n, ok := keyNames[name]; ok {
		name = n
	}
	b.WriteString(name)
	return b.String()
}

// keysOf returns how the keys of the commands named are written, each
// once, separated by " / ".
func keysOf(names ...string) string {
	var out []string
	for _, name := range names {
		i := slices.IndexFunc(commands, func(c command) bool { return c.name == name })
		if i < 0 {
			panic(fmt.Sprintf("no command %q", name))
		}
		for _, k := range commands[i].keys {
			if s := "**" + k.String() + "**"; !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return strings.Join(out, " / ")
}

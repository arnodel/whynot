//go:build unix

package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIKeyFrom(t *testing.T) {
	write := func(t *testing.T, content string, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil { // past the umask
			t.Fatal(err)
		}
		return path
	}
	for _, c := range []struct {
		name, content string
		mode          os.FileMode
		key, err      string
	}{
		{"a key", "anthropic-api-key: sk-test\n", 0o600, "sk-test", ""},
		{"read-only", "anthropic-api-key: sk-test\n", 0o400, "sk-test", ""},
		{"a command", "anthropic-api-key-command: echo '  sk-cmd  '\n", 0o600, "sk-cmd", ""},
		{"no key", "other: 1\n", 0o644, "", ""},
		{"readable by others", "anthropic-api-key: sk-test\n", 0o644, "", "chmod 600"},
		{"a command writable by others", "anthropic-api-key-command: echo sk\n", 0o620, "", "chmod 600"},
		{"both", "anthropic-api-key: a\nanthropic-api-key-command: echo b\n", 0o600, "", "keep one"},
		{"a failing command", "anthropic-api-key-command: echo oops >&2; exit 3\n", 0o600, "", "oops"},
		{"a silent command", "anthropic-api-key-command: 'true'\n", 0o600, "", "printed nothing"},
	} {
		t.Run(c.name, func(t *testing.T) {
			key, _, err := apiKeyFrom(write(t, c.content, c.mode))
			if key != c.key {
				t.Errorf("the key is %q, want %q", key, c.key)
			}
			if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
				t.Errorf("the error is %v, want one saying %q", err, c.err)
			}
		})
	}
	if key, _, err := apiKeyFrom(filepath.Join(t.TempDir(), "none.yaml")); key != "" || err != nil {
		t.Errorf("a missing file gives %q, %v; want no key and no error", key, err)
	}
}

func TestConfigPath(t *testing.T) {
	xdg, home := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("HOME", home)
	if path, _ := configPath(); path != "" {
		t.Errorf("with no config file, the path is %q", path)
	}
	system, err := os.UserConfigDir()
	if err != nil {
		t.Skip(err)
	}
	write := func(dir string) string {
		p := filepath.Join(dir, "whynot", "config.yaml")
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, nil, 0o600)
		return p
	}
	sys := write(system)
	if path, ignored := configPath(); path != sys || len(ignored) != 0 {
		t.Errorf("the path is %q, ignoring %v; want %q", path, ignored, sys)
	}
	x := write(xdg)
	path, ignored := configPath()
	if path != x {
		t.Errorf("the path is %q, want %q: $XDG_CONFIG_HOME comes first", path, x)
	}
	if sys != x && (len(ignored) != 1 || ignored[0] != sys) {
		t.Errorf("ignored %v, want %q", ignored, sys)
	}
}

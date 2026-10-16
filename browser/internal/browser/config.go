package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// config is the viewers' config file, config.yaml in a whynot directory
// in the user's config directory (see os.UserConfigDir). It's read by the
// command-line viewers and by the app bundles alike, which don't get the
// shell's environment.
type config struct {
	// AnthropicAPIKey is the key for the AI. AnthropicAPIKeyCommand is a
	// command that prints it instead, such as
	//
	//	security find-generic-password -s whynot-claude-api-key -w
	//
	// so that the key can stay in a password store.
	AnthropicAPIKey        string `yaml:"anthropic-api-key"`
	AnthropicAPIKeyCommand string `yaml:"anthropic-api-key-command"`
}

// AnthropicAPIKey returns the key for the AI, and where it came from:
// ANTHROPIC_API_KEY if it's set, or else the config file. It returns ""
// without an error if neither has one. The config file is only trusted
// with a key, or with a command, if only its owner can read and write it,
// as with chmod 600.
func AnthropicAPIKey() (key, from string, err error) {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key, "ANTHROPIC_API_KEY", nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", "", nil
	}
	return apiKeyFrom(filepath.Join(dir, "whynot", "config.yaml"))
}

// apiKeyFrom is AnthropicAPIKey for the config file at path.
func apiKeyFrom(path string) (key, from string, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	var c config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return "", "", fmt.Errorf("reading %s: %w", path, err)
	}
	if c.AnthropicAPIKey == "" && c.AnthropicAPIKeyCommand == "" {
		return "", "", nil
	}
	if c.AnthropicAPIKey != "" && c.AnthropicAPIKeyCommand != "" {
		return "", "", fmt.Errorf("%s has both anthropic-api-key and anthropic-api-key-command: keep one", path)
	}
	if err := checkPrivate(path); err != nil {
		return "", "", err
	}
	if c.AnthropicAPIKey != "" {
		return strings.TrimSpace(c.AnthropicAPIKey), path, nil
	}
	key, err = runKeyCommand(c.AnthropicAPIKeyCommand)
	if err != nil {
		return "", "", fmt.Errorf("anthropic-api-key-command in %s: %w", path, err)
	}
	return key, "anthropic-api-key-command in " + path, nil
}

// runKeyCommand runs command with the system's shell, and returns what it
// printed. A password store may ask the user first, so it has a minute.
func runKeyCommand(command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	key := strings.TrimSpace(string(out))
	if key == "" {
		return "", errors.New("it printed nothing")
	}
	return key, nil
}

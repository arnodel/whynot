package whymark

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// settings are what a document's front matter says.
type settings struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Whymark     *struct {
		Version int    `yaml:"version"`
		Start   string `yaml:"start"`
		Journal bool   `yaml:"journal"`
	} `yaml:"whymark"`
}

// version is the version of the format this engine understands.
const version = 1

// frontMatter returns the front matter at the start of source, between
// "---" lines, and the number of lines it takes, delimiters included. ok
// is false if source has none.
func frontMatter(source []byte) (front []byte, lines int, ok bool) {
	rest, found := bytes.CutPrefix(source, []byte("---\n"))
	if !found {
		if rest, found = bytes.CutPrefix(source, []byte("---\r\n")); !found {
			return nil, 0, false
		}
	}
	for i := 1; len(rest) > 0; i++ {
		line, after, _ := bytes.Cut(rest, []byte("\n"))
		if d := bytes.TrimRight(line, "\r"); string(d) == "---" || string(d) == "..." {
			return front, i + 1, true
		}
		front = append(front, line...)
		front = append(front, '\n')
		rest = after
	}
	return nil, 0, false
}

// readSettings returns the settings in source's front matter.
func readSettings(source []byte) (settings, error) {
	var s settings
	front, _, ok := frontMatter(source)
	if !ok {
		return s, nil
	}
	if err := yaml.Unmarshal(front, &s); err != nil {
		return s, fmt.Errorf("front matter: %w", err)
	}
	return s, nil
}

// IsDocument reports whether source is a whymark document: Markdown whose
// front matter has a whymark key.
func IsDocument(source []byte) bool {
	s, err := readSettings(source)
	return err == nil && s.Whymark != nil
}

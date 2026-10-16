package compile

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Settings are what a document's front matter says.
type Settings struct {
	// Title and Description are the document's, from the top level of its
	// front matter.
	Title, Description string
	// Version is the version of the format the document is written for.
	Version int
	// Start is the id of the passage the document starts at, if not the
	// first.
	Start string
	// Journal is a hint that the document reads best as a trail.
	Journal bool
	// Resume is a hint that opening the document again should carry on
	// from where its reader left it.
	Resume bool
	// Uses is the capabilities the document declares, by name.
	Uses map[string]Need
}

// Need is how a document needs a capability it declares.
type Need int

const (
	// Optional is a capability the document works without, better with.
	Optional Need = iota
	// Required is one it can't work without.
	Required
)

// Version is the version of the format this compiler understands.
const Version = 1

// frontMatter is a document's front matter, as YAML.
type frontMatter struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Whymark     *struct {
		Version int               `yaml:"version"`
		Start   string            `yaml:"start"`
		Journal bool              `yaml:"journal"`
		Resume  bool              `yaml:"resume"`
		Uses    map[string]string `yaml:"uses"`
	} `yaml:"whymark"`
}

// splitFrontMatter returns the front matter at the start of source,
// between "---" lines, and the number of lines it takes, delimiters
// included. ok is false if source has none.
func splitFrontMatter(source []byte) (front []byte, lines int, ok bool) {
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

// ReadSettings returns the settings in the front matter of source, a
// whymark document. It's an error if source isn't one: Markdown whose
// front matter has a whymark key.
func ReadSettings(source []byte) (Settings, error) {
	front, _, ok := splitFrontMatter(source)
	if !ok {
		return Settings{}, errNotDocument
	}
	var fm frontMatter
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return Settings{}, fmt.Errorf("front matter: %w", err)
	}
	if fm.Whymark == nil {
		return Settings{}, errNotDocument
	}
	uses := map[string]Need{}
	for name, need := range fm.Whymark.Uses {
		switch need {
		case "optional":
			uses[name] = Optional
		case "required":
			uses[name] = Required
		default:
			return Settings{}, fmt.Errorf("front matter: uses: %s is %q: a capability is optional or required", name, need)
		}
	}
	return Settings{
		Title:       fm.Title,
		Description: fm.Description,
		Version:     fm.Whymark.Version,
		Start:       fm.Whymark.Start,
		Journal:     fm.Whymark.Journal,
		Resume:      fm.Whymark.Resume,
		Uses:        uses,
	}, nil
}

var errNotDocument = fmt.Errorf("not a whymark document: its front matter has no whymark key")

// IsDocument reports whether source is a whymark document.
func IsDocument(source []byte) bool {
	_, err := ReadSettings(source)
	return err == nil
}

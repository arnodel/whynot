package browser

import "go.yaml.in/yaml/v3"

// frontMatterTitle returns the title that raw, a document's front matter
// (see whynot.Document.RawFrontMatter), gives, or "" if it gives none or
// isn't YAML.
func frontMatterTitle(raw string) string {
	var fm struct {
		Title string `yaml:"title"`
	}
	if err := yaml.Unmarshal([]byte(raw), &fm); err != nil {
		return ""
	}
	return fm.Title
}

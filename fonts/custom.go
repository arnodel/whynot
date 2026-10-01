package fonts

import (
	"fmt"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// CustomSelector is a FaceSelector backed by caller-supplied TTF/OTF
// font bytes, registered per (Family, weight-bucket, style) slot via
// AddFont. Any (family, weight, style) combination with nothing registered
// is delegated to a fallback FaceSelector - a GoSelector by default,
// overridable via WithFallback - so callers only need to supply the fonts
// they actually care about overriding. WithFallback(nil) means "no
// fallback": SelectFace returns an error for any unfilled slot instead.
type CustomSelector struct {
	fonts    map[TextStyle]*opentype.Font // keyed by fontKey
	faces    map[faceKey]font.Face
	fallback FaceSelector
}

var _ FaceSelector = (*CustomSelector)(nil)

// CustomOption customizes a CustomSelector at construction, via
// NewCustomSelector's opts parameter.
type CustomOption func(*CustomSelector)

// WithFallback overrides the FaceSelector NewCustomSelector otherwise
// defaults to (a GoSelector) for any (family, weight, style) slot nothing
// was registered for. Pass nil to disable fallback entirely - SelectFace
// then returns an error for an unfilled slot instead.
func WithFallback(fallback FaceSelector) CustomOption {
	return func(s *CustomSelector) {
		s.fallback = fallback
	}
}

// NewCustomSelector returns a CustomSelector with nothing registered yet -
// every SelectFace call delegates to its fallback FaceSelector (a
// GoSelector unless overridden via WithFallback) until AddFont/AddFontFile
// is called.
func NewCustomSelector(opts ...CustomOption) *CustomSelector {
	s := &CustomSelector{
		fonts:    map[TextStyle]*opentype.Font{},
		faces:    map[faceKey]font.Face{},
		fallback: NewGoSelector(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// AddFont registers data (TTF/OTF bytes, or a .ttc/.otc collection) for
// exactly the (family, weight, style) slot given, replacing whatever was
// registered for that slot before. index picks which subfont within data
// to use - 0 for an ordinary single-font file (opentype.ParseCollection
// treats it as a 1-font collection), or a specific subfont's index within
// a known .ttc/.otc. weight and style are bucketed exactly like a
// resolved TextStyle is in SelectFace (see fontKey).
//
// data is parsed immediately, so a malformed font or an out-of-range
// index is reported here, at registration time, rather than surfacing
// later from SelectFace. On error nothing is registered and any
// already-cached faces are left untouched.
//
// Use AddFontCollection instead to register every subfont a collection
// contains that can be confidently classified, rather than picking one by
// index yourself.
func (s *CustomSelector) AddFont(family Family, weight font.Weight, style font.Style, data []byte, index int) error {
	collection, err := opentype.ParseCollection(data)
	if err != nil {
		return fmt.Errorf("fonts: parsing font: %w", err)
	}
	parsed, err := collection.Font(index)
	if err != nil {
		return fmt.Errorf("fonts: parsing font: selecting subfont %d: %w", index, err)
	}
	key := fontKey(TextStyle{Weight: weight, Style: style, Family: family})
	s.fonts[key] = parsed
	// A face rasterized from the old (or absent) font for this slot may
	// already be cached at some size - drop every cached face so the next
	// SelectFace call re-rasterizes from what was just registered. AddFont
	// is meant to be called during setup, before rendering starts, so this
	// is not a hot-path concern.
	s.faces = map[faceKey]font.Face{}
	return nil
}

// AddFontFile is AddFont, reading data from path instead of taking it
// directly. A convenience for the common case of loading a font straight
// from disk; font bytes coming from anywhere else (embed.FS, a network
// fetch) still go through AddFont directly.
func (s *CustomSelector) AddFontFile(family Family, weight font.Weight, style font.Style, path string, index int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("fonts: reading font file: %w", err)
	}
	return s.AddFont(family, weight, style, data, index)
}

// AddFontCollection registers every subfont in data (a .ttc/.otc
// collection, or a plain single-font TTF/OTF) that classifies cleanly
// into a (weight, style) slot for family, via each subfont's own
// name-table subfamily string (see classifySubfamily in
// classify.go). If matchFamily is non-empty, a subfont is also
// required to have its own name-table family reasonably match it
// (case/space/punctuation-insensitive, either direction as a substring -
// see subfontBelongsToFamily) - pass "" to register every classifiable
// subfont regardless of its own family name, e.g. when data is already
// known to hold only the one family wanted.
//
// Anything unrecognized (Light/Thin/Black/SemiBold/Condensed/... - see
// classifySubfamily's doc comment for why these are skipped rather than
// guessed) or family-mismatched is simply not registered, not an error -
// those slots are served by the fallback FaceSelector instead. The only
// error this returns is a failure to parse data at all; it returns how
// many subfonts were registered otherwise.
func (s *CustomSelector) AddFontCollection(family Family, data []byte, matchFamily string) (registered int, err error) {
	collection, err := opentype.ParseCollection(data)
	if err != nil {
		return 0, fmt.Errorf("fonts: parsing font: %w", err)
	}
	var buf sfnt.Buffer
	for i := 0; i < collection.NumFonts(); i++ {
		sub, err := collection.Font(i)
		if err != nil {
			continue
		}
		if !subfontBelongsToFamily(sub, &buf, matchFamily) {
			continue
		}
		subfamily := subfontName(sub, &buf, sfnt.NameIDTypographicSubfamily, sfnt.NameIDSubfamily)
		weight, style, ok := classifySubfamily(subfamily)
		if !ok {
			continue
		}
		if err := s.AddFont(family, weight, style, data, i); err != nil {
			continue
		}
		registered++
	}
	return registered, nil
}

// AddFontCollectionFile is AddFontCollection, reading data from path
// instead of taking it directly.
func (s *CustomSelector) AddFontCollectionFile(family Family, path string, matchFamily string) (registered int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("fonts: reading font file: %w", err)
	}
	return s.AddFontCollection(family, data, matchFamily)
}

func (s *CustomSelector) SelectFace(style TextStyle, dpi float64) (font.Face, error) {
	key := faceKey{style, dpi}
	if face, ok := s.faces[key]; ok {
		return face, nil
	}
	slot := fontKey(style)
	parsed, ok := s.fonts[slot]
	if !ok {
		if s.fallback == nil {
			return nil, fmt.Errorf("fonts: no font registered for %+v and no fallback selector configured", slot)
		}
		return s.fallback.SelectFace(style, dpi)
	}
	face, err := newFace(parsed, style, dpi)
	if err != nil {
		return nil, err
	}
	s.faces[key] = face
	return face, nil
}

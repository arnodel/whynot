package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// FileResolver resolves file: URLs to the files beneath Root, and only
// those: a path that leads outside Root, by "..", an absolute path or a
// symbolic link, fails to fetch. A FileResolver with a nil Root resolves
// nothing, so allowing any file has to be explicit:
//
//	root, err := os.OpenRoot("/")
//
// Its keys are cleaned absolute paths, so one file reached through two
// relative paths is cached once. Its Sources' media type comes from the
// file's extension.
type FileResolver struct {
	Root *os.Root
}

func (FileResolver) Schemes() []string { return []string{"file"} }

func (r FileResolver) Resolve(u *url.URL) (Source, error) {
	if r.Root == nil {
		return nil, errors.New("fetch: FileResolver has no Root")
	}
	path, err := filepath.Abs(urlFilePath(u))
	if err != nil {
		return nil, err
	}
	rootDir, err := filepath.Abs(r.Root.Name())
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(rootDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("fetch: %s is outside %s", path, rootDir)
	}
	return fileSource{root: r.Root, rel: rel, path: path}, nil
}

// urlFilePath is the local file path of a file: URL.
func urlFilePath(u *url.URL) string {
	p := u.Path
	if p == "" {
		p = u.Opaque
	}
	// A Windows path in a URL has a slash before its drive: /C:/dir.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

type fileSource struct {
	root *os.Root
	rel  string // the path within root
	path string // the absolute path
}

func (s fileSource) Key() string { return filepath.ToSlash(s.path) }

func (s fileSource) Fetch(context.Context) (io.ReadCloser, string, error) {
	f, err := s.root.Open(s.rel)
	if err != nil {
		return nil, "", err
	}
	return f, mediaTypeByExtension(s.path), nil
}

// extensionTypes are media types missing from some systems' tables.
var extensionTypes = map[string]string{
	".md":       "text/markdown",
	".markdown": "text/markdown",
}

// mediaTypeByExtension is the media type of a file named path, without
// parameters, or "" if its extension is unknown.
func mediaTypeByExtension(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if t, ok := extensionTypes[ext]; ok {
		return t
	}
	return mediaType(mime.TypeByExtension(ext))
}

// mediaType is t without its parameters, or "" if t can't be parsed.
func mediaType(t string) string {
	mt, _, err := mime.ParseMediaType(t)
	if err != nil {
		return ""
	}
	return mt
}

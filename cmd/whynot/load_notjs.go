//go:build !js

package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// absFileURL turns a command-line path into a file: URL with an
// absolute path, so it can be used as the base for resolving a
// relative link the same way an http(s) URL would be.
func absFileURL(path string) (*url.URL, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return &url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}, nil
}

// resolveLocationArg turns text - a command-line argument, or pasted
// clipboard content (see game.paste) - into a location to load: the
// word "welcome" for the built-in welcome page, an http(s) URL parsed
// as-is, or an existing local file path turned into an absolute file:
// URL - in that order, so e.g. a URL is never misread as a file path
// the way absFileURL alone would (filepath.Abs happily "resolves" any
// string, URLs included, against the working directory).
func resolveLocationArg(text string) (*url.URL, error) {
	if strings.EqualFold(text, "welcome") {
		return welcomeURL, nil
	}
	if u, err := url.Parse(text); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return u, nil
	}
	if _, err := os.Stat(text); err == nil {
		return absFileURL(text)
	}
	return nil, fmt.Errorf("%q isn't \"welcome\", a URL, or an existing file path", text)
}

// loadDocument fetches the bytes at location - a local read for a
// file: URL, an HTTP GET for http(s) (see fetchDocument), the embedded
// page for welcomeURL. Any other scheme (e.g. a mailto: autolink) is
// rejected rather than misread as a file path.
func loadDocument(location *url.URL) ([]byte, error) {
	switch location.Scheme {
	case "whynot":
		return renderWelcome(), nil
	case "http", "https":
		return fetchDocument(location)
	case "file", "":
		return os.ReadFile(location.Path)
	default:
		return nil, fmt.Errorf("unsupported link scheme %q", location.Scheme)
	}
}

// openImageLocation fetches the bytes at location - the same file-or-
// http(s) rule loadDocument uses (see fetchImage).
func openImageLocation(location *url.URL) (io.ReadCloser, error) {
	switch location.Scheme {
	case "http", "https":
		return fetchImage(location)
	case "file", "":
		return os.Open(location.Path)
	default:
		return nil, fmt.Errorf("unsupported image scheme %q", location.Scheme)
	}
}

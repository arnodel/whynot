//go:build !unix

package browser

// checkPrivate is a no-op where files have no Unix permissions: on
// Windows, a file in the user's profile is private to them by default.
func checkPrivate(path string) error { return nil }

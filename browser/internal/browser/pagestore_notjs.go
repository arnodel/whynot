//go:build !js

package browser

import (
	"path/filepath"

	"github.com/arnodel/whynot/browser/internal/whymark"
)

// openPageStore opens the store of whymark pages in dir: a SQLite
// database.
func openPageStore(dir string) (whymark.Store, error) {
	return whymark.OpenSQLiteStore(filepath.Join(dir, "whymark.db"))
}

//go:build unix

package browser

import (
	"fmt"
	"os"
	"syscall"
)

// checkPrivate returns an error unless only its owner, the user, can read
// and write the file at path.
func checkPrivate(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s belongs to another user: it isn't trusted with the AI's key", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("others can read or write %s, so it isn't trusted with the AI's key: run chmod 600 %s", path, path)
	}
	return nil
}

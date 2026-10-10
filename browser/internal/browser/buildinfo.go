package browser

import (
	"fmt"
	"runtime/debug"
	"time"
)

// vcsBuild is the commit a binary was built from, as the go command
// records it when building from a git checkout.
type vcsBuild struct {
	revision string // the full commit hash
	time     time.Time
	modified bool // built with uncommitted changes
}

// readVCSBuild returns the commit this binary was built from, or ok=false
// if it wasn't built from a git checkout (as with go install pkg@version).
func readVCSBuild() (vcsBuild, bool) {
	info, _ := debug.ReadBuildInfo()
	return vcsBuildFrom(info)
}

// vcsBuildFrom is readVCSBuild for the given build info, which may be nil.
func vcsBuildFrom(info *debug.BuildInfo) (b vcsBuild, ok bool) {
	if info == nil {
		return vcsBuild{}, false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.revision = s.Value
		case "vcs.time":
			b.time, _ = time.Parse(time.RFC3339, s.Value)
		case "vcs.modified":
			b.modified = s.Value == "true"
		}
	}
	return b, b.revision != ""
}

// versionFrom is the version to show for a binary, from the first source
// that knows it: stamped, the version a release build is given at link
// time ("dev" when it isn't); the module version the go command records
// for go install pkg@version; the commit, for a build from a git
// checkout; or else "dev". info may be nil.
func versionFrom(stamped string, info *debug.BuildInfo) string {
	if stamped != "dev" {
		return "v" + stamped
	}
	// A build from a checkout records "(devel)" here, or nothing.
	if info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	if b, ok := vcsBuildFrom(info); ok {
		return "dev " + b.short()
	}
	return "dev"
}

// short is the commit's abbreviated hash, as git shows it.
func (b vcsBuild) short() string {
	if len(b.revision) > 7 {
		return b.revision[:7]
	}
	return b.revision
}

// line is the welcome page's line about the build: the commit, linked,
// its time, and whether there were local changes.
func (b vcsBuild) line() string {
	s := fmt.Sprintf("*Built from commit [%s](https://github.com/arnodel/whynot/commit/%s)", b.short(), b.revision)
	if !b.time.IsZero() {
		s += ", of " + b.time.UTC().Format("2 Jan 2006, 15:04 MST")
	}
	if b.modified {
		s += ", with local changes"
	}
	return s + ".*\n\n"
}

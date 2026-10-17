// Command macapp makes a macOS app bundle around a viewer's binary:
//
//	go run ./internal/macapp -binary whynot -name WhyNot -id io.github.arnodel.whynot -out build
//
// makes build/WhyNot.app. The bundle is only files, so it can be made on
// any system, as releases are; on macOS it's also signed ad hoc, with
// codesign. Running as a bundle gives the app its name and icon in the
// Dock and the menu bar, and the usage descriptions macOS needs before
// letting it use the microphone and speech recognition.
package main

import (
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	binary := flag.String("binary", "", "the viewer's binary, built for macOS")
	name := flag.String("name", "", "the app's name, as the Dock and the menu bar show it")
	id := flag.String("id", "", "the bundle identifier, such as io.github.arnodel.whynot")
	version := flag.String("version", "0.0.0", "the app's version")
	icon := flag.String("icon", "", "a square PNG for the app's icon, ideally 1024 pixels wide (optional)")
	out := flag.String("out", ".", "the directory to make the bundle in")
	flag.Parse()
	if *binary == "" || *name == "" || *id == "" {
		flag.Usage()
		os.Exit(2)
	}
	app := filepath.Join(*out, *name+".app")
	if err := makeBundle(app, bundle{
		binary: *binary, name: *name, id: *id, version: *version, icon: *icon,
	}); err != nil {
		log.Fatal(err)
	}
	fmt.Println(app)
}

// A bundle is what makeBundle makes an app bundle from.
type bundle struct {
	binary, name, id, version string
	icon                      string // a PNG file, or ""
}

// makeBundle makes the app bundle b at app, replacing any already there.
func makeBundle(app string, b bundle) error {
	if err := os.RemoveAll(app); err != nil {
		return err
	}
	contents := filepath.Join(app, "Contents")
	executable := filepath.Base(b.binary)
	if err := copyFile(filepath.Join(contents, "MacOS", executable), b.binary, 0o755); err != nil {
		return err
	}
	iconFile := ""
	if b.icon != "" {
		iconFile = "icon.icns"
		if err := writeIcon(filepath.Join(contents, "Resources", iconFile), b.icon); err != nil {
			return fmt.Errorf("making the icon: %w", err)
		}
	}
	plist := infoPlist(b.name, b.id, executable, b.version, iconFile)
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), plist, 0o644); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		// Signed whole, so that macOS takes the bundle's identity; the Go
		// linker signs the binary alone.
		if out, err := exec.Command("codesign", "--force", "--sign", "-", app).CombinedOutput(); err != nil {
			return fmt.Errorf("signing %s: %v\n%s", app, err, out)
		}
	}
	return nil
}

func copyFile(dst, src string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

// writeIcon writes the PNG at src to dst as an icon file, at the sizes the
// Dock and Finder use.
func writeIcon(dst, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	data, err := icns(img)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

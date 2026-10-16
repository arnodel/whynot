package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMakeBundle(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "whynot")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	iconPNG := filepath.Join(dir, "icon.png")
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for i := range 100 {
		img.Set(i, i, color.NRGBA{R: 255, A: 255})
	}
	f, err := os.Create(iconPNG)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, img)
	f.Close()

	app := filepath.Join(dir, "out", "Why <Not>.app")
	err = makeBundle(app, bundle{binary: bin, name: "Why <Not>", id: "io.example.whynot", version: "v1.2.3-rc.1", icon: iconPNG})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(app, "Contents", "MacOS", "whynot"))
	if err != nil || info.Mode()&0o111 == 0 {
		t.Errorf("the bundle's executable: %v, mode %v", err, info.Mode())
	}
	plist, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<string>io.example.whynot</string>", "<string>Why &lt;Not&gt;</string>",
		"<string>1.2.3</string>", "<string>icon.icns</string>", "<string>whynot</string>",
	} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("Info.plist lacks %s:\n%s", want, plist)
		}
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("plutil", "-lint", filepath.Join(app, "Contents", "Info.plist")).CombinedOutput(); err != nil {
			t.Errorf("plutil: %v\n%s", err, out)
		}
	}

	icon, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "icon.icns"))
	if err != nil {
		t.Fatal(err)
	}
	if string(icon[:4]) != "icns" || int(binary.BigEndian.Uint32(icon[4:8])) != len(icon) {
		t.Fatalf("the icon file's header is %q, length %d, for %d bytes", icon[:4], binary.BigEndian.Uint32(icon[4:8]), len(icon))
	}
	var kinds []string
	for rest := icon[8:]; len(rest) > 0; {
		n := int(binary.BigEndian.Uint32(rest[4:8]))
		if n < 8 || n > len(rest) {
			t.Fatalf("entry %q has length %d, with %d bytes left", rest[:4], n, len(rest))
		}
		if _, err := png.Decode(strings.NewReader(string(rest[8:n]))); err != nil {
			t.Errorf("entry %q isn't a PNG: %v", rest[:4], err)
		}
		kinds = append(kinds, string(rest[:4]))
		rest = rest[n:]
	}
	if len(kinds) != len(icnsSizes) {
		t.Errorf("the icon file has entries %v", kinds)
	}
}

func TestBundleVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v1.0.0-rc.3": "1.0.0", "1.2": "1.2", "2.0.1": "2.0.1", "dev": "0.0.0", "": "0.0.0",
	} {
		if got := bundleVersion(in); got != want {
			t.Errorf("bundleVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

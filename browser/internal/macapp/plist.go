package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"regexp"
)

// infoPlist returns an app bundle's Info.plist. iconFile, in the bundle's
// Resources, may be "".
func infoPlist(name, id, executable, version, iconFile string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	key := func(k, v string) {
		fmt.Fprintf(&b, "\t<key>%s</key>\n\t<string>", k)
		xml.EscapeText(&b, []byte(v))
		b.WriteString("</string>\n")
	}
	numeric := bundleVersion(version)
	key("CFBundleName", name)
	key("CFBundleDisplayName", name)
	key("CFBundleIdentifier", id)
	key("CFBundleExecutable", executable)
	key("CFBundlePackageType", "APPL")
	key("CFBundleShortVersionString", numeric)
	key("CFBundleVersion", numeric)
	if iconFile != "" {
		key("CFBundleIconFile", iconFile)
	}
	// Go 1.25's minimum.
	key("LSMinimumSystemVersion", "12.0")
	key("NSMicrophoneUsageDescription", "Dictation into the prompt.")
	b.WriteString("\t<key>NSHighResolutionCapable</key>\n\t<true/>\n</dict>\n</plist>\n")
	return b.Bytes()
}

var versionNumbers = regexp.MustCompile(`^v?(\d+(\.\d+){0,2})`)

// bundleVersion returns version as macOS wants it in a bundle: up to three
// numbers, so "v1.0.0-rc.3" is "1.0.0".
func bundleVersion(version string) string {
	if m := versionNumbers.FindStringSubmatch(version); m != nil {
		return m[1]
	}
	return "0.0.0"
}

# Local builds of the viewers as macOS apps: "make mac-app" makes
# build/WhyNot.app, from cmd/whynot, and "make mac-app-gio" makes
# build/giowhynot.app, from cmd/giowhynot. Open them with
# "open build/WhyNot.app", or run build/WhyNot.app/Contents/MacOS/whynot to
# see the log in the terminal.

BUILD := build
ICON := internal/macapp/icon.png
# The latest release tag, without its "v": the bundle shows its numbers.
VERSION := $(patsubst v%,%,$(shell git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null))

.PHONY: mac-app mac-app-gio clean

mac-app:
	go build -o $(BUILD)/bin/whynot ./cmd/whynot
	go run ./internal/macapp -binary $(BUILD)/bin/whynot -name WhyNot -id io.github.arnodel.whynot -version "$(VERSION)" -icon $(ICON) -out $(BUILD)

mac-app-gio:
	go build -o $(BUILD)/bin/giowhynot ./cmd/giowhynot
	go run ./internal/macapp -binary $(BUILD)/bin/giowhynot -name giowhynot -id io.github.arnodel.giowhynot -version "$(VERSION)" -icon $(ICON) -out $(BUILD)

clean:
	rm -rf $(BUILD)

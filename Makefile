.PHONY: build build-windows embed-rules icon run release-windows release-linux release-macos release-all clean

# Copies the maintainer-built rules database into the app's embed source.
# Run after any n5e-ingest step that touches out/rules.db.
# internal/embedded/rules.db is a build artifact, not source — gitignored
# the same way out/rules.db already is.
embed-rules:
	cp out/rules.db internal/embedded/rules.db

build: embed-rules
	go build -o dist/n5e ./cmd/n5e
	go build -o dist/n5e-ingest ./cmd/n5e-ingest

# Regenerates cmd/n5e/resource_windows.syso (icon + version info) from
# assets/n5e.ico via cmd/n5e/versioninfo.json — see cmd/n5e/main.go's
# go:generate directive. Skipped with a warning, not a hard failure, if the
# icon hasn't been supplied yet (assets/n5e.ico is not committed to the
# repo — supply your own .ico there before shipping a real release build).
icon:
	@if [ -f assets/n5e.ico ]; then \
		go generate ./cmd/n5e; \
	else \
		echo "warning: assets/n5e.ico not found — building without an app icon (see Makefile's icon target)"; \
	fi

# Windows GUI-subsystem build: suppresses the console window so the shipped
# app has zero CLI surface for end users. n5e-ingest stays a normal console
# binary — it's maintainer-only tooling, never shipped to players.
build-windows: embed-rules icon
	GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui" -o dist/n5e.exe ./cmd/n5e
	GOOS=windows GOARCH=amd64 go build -o dist/n5e-ingest.exe ./cmd/n5e-ingest

run: build
	./dist/n5e

# Zips the player-facing Windows exe into a "download, unzip, double-click"
# package — no installer, matching the single-portable-exe distribution
# model. n5e-ingest is maintainer-only tooling and deliberately left out of
# the release zip. Uses tools/zipdist (stdlib archive/zip) instead of a
# system `zip` binary, since that isn't guaranteed to be installed.
# Override the output filename with ZIPNAME, e.g.:
#   make release-windows ZIPNAME=n5e-v2.3-windows.zip
release-windows: ZIPNAME ?= n5e-windows.zip
release-windows: build-windows
	mkdir -p dist-release
	go run ./tools/zipdist "dist-release/$(ZIPNAME)" dist/n5e.exe

# Linux has no PE-resource-style mechanism to embed an icon in a bare ELF
# binary (that's a Windows-specific concept — see the icon target above),
# so assets/n5e.ico rides along loose in the zip instead. n5e-linux needs
# its executable bit set explicitly before zipping — a build's default
# output permissions aren't guaranteed executable on every platform, and
# zipdist now faithfully carries whatever mode the file has into the
# archive (see tools/zipdist's FileInfoHeader fix), so this has to be
# right going in.
# Override the output filename with ZIPNAME, e.g.:
#   make release-linux ZIPNAME=n5e-v2.3-linux.zip
release-linux: ZIPNAME ?= n5e-linux.zip
release-linux: embed-rules
	mkdir -p dist-release
	GOOS=linux GOARCH=amd64 go build -o dist/n5e-linux ./cmd/n5e
	chmod +x dist/n5e-linux
	go run ./tools/zipdist "dist-release/$(ZIPNAME)" dist/n5e-linux assets/n5e.ico

# No lipo/universal-binary support when cross-compiling from Linux, so
# Apple Silicon and Intel ship as two separate binaries in the same zip
# rather than one fat one — whoever's testing picks the one matching
# their Mac. Same "no bare-executable icon embedding" situation as Linux
# above (macOS icons normally live in an .app bundle's Resources/*.icns,
# well beyond what a quick dev-testing binary needs) — n5e.ico rides along
# loose here too.
# Stripped with -ldflags="-s -w" (drops the debug symbol table and DWARF
# info, neither used at runtime) — shipping both arches unstripped puts the
# zip over GitHub's 25MB file-size limit.
# Override the output filename with ZIPNAME, e.g.:
#   make release-macos ZIPNAME=n5e-v2.3-macos.zip
release-macos: ZIPNAME ?= n5e-macos.zip
release-macos: embed-rules
	mkdir -p dist-release
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/n5e-macos-arm64 ./cmd/n5e
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/n5e-macos-amd64 ./cmd/n5e
	chmod +x dist/n5e-macos-arm64 dist/n5e-macos-amd64
	go run ./tools/zipdist "dist-release/$(ZIPNAME)" dist/n5e-macos-arm64 dist/n5e-macos-amd64 assets/n5e.ico

# Builds every shipped OS's release zip in one shot, named
# n5e-<os>-<name>.zip (see tools/release/release-all.rb, which this just
# forwards to). Requires NAME=, not -n: -n is GNU Make's own dry-run flag
# and can't be forwarded through from here — run the script directly
# instead if you want that:
#   ruby tools/release/release-all.rb -n v0.2.1
release-all:
	@if [ -z "$(NAME)" ]; then echo "usage: make release-all NAME=v0.2.1"; exit 1; fi
	ruby tools/release/release-all.rb -n "$(NAME)"

clean:
	rm -rf dist dist-release internal/embedded/rules.db cmd/n5e/resource_windows.syso

# One command surface for the repo. Run `just` to list recipes.
set shell := ["bash", "-euo", "pipefail", "-c"]

import "tools/mutation/mutation.just"

md_paths := "docs contracts AGENTS.md README.md"

default:
    @just --list

# Build landline, landline-switchboard, and landline-mcp into ./bin, and landline-menubar on macOS.
build: (binaries "bin")

# Build and install the binaries into ~/.local/bin, or into prefix/bin when one is given. On macOS it also installs
# Landline Menubar.app into ~/Applications (see install-app).
install prefix="~/.local": (binaries prefix + "/bin")
    @echo "installed landline, landline-switchboard, and landline-mcp in {{ prefix }}/bin"
    @if [ "$(uname -s)" = Darwin ]; then echo "installed landline-menubar in {{ prefix }}/bin"; just install-app; fi

# macOS only: build a stamped landline-menubar into Landline Menubar.app, ad hoc sign it, and install it into folder
# (default ~/Applications, which Spotlight indexes and needs no administrator password). An existing bundle of the same
# name is replaced after the new one is built and signed. The icon is made from assets/jack.svg on every run.
install-app folder="~/Applications":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ "$(uname -s)" != Darwin ]; then echo "skipping Landline Menubar.app: macOS only"; exit 0; fi
    folder={{ folder }}
    mkdir -p "$folder"
    folder="$(cd "$folder" && pwd)"
    version="$(tr -d '[:space:]' < VERSION)"
    stamp="$(tools/stamp/stamp.sh)"; commit="${stamp#commit=}"; commit="${commit%%$'\n'*}"; date="${stamp##*date=}"
    flags="-X main.version=$version"
    if [ -n "$commit" ]; then flags="$flags -X main.commit=$commit"; fi
    if [ -n "$date" ]; then flags="$flags -X main.date=$date"; fi
    build="${commit:0:7}"; build="${build:-$version}"
    work="$(mktemp -d "$folder/.landline-menubar.XXXXXX")"
    trap 'rm -rf "$work"' EXIT
    app="$work/Landline Menubar.app"
    mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$work/AppIcon.iconset"
    unset GOROOT
    (cd menubar && CGO_ENABLED=1 CC="${CC:-/usr/bin/clang}" go build -trimpath -ldflags "$flags" \
      -o "$app/Contents/MacOS/landline-menubar" .)
    (cd menubar && go run ./tools/icons -iconset ../assets/jack.svg "$work/AppIcon.iconset")
    iconutil -c icns -o "$app/Contents/Resources/AppIcon.icns" "$work/AppIcon.iconset"
    cat > "$app/Contents/Info.plist" <<PLIST
    <?xml version="1.0" encoding="UTF-8"?>
    <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
    <plist version="1.0">
    <dict>
      <key>CFBundleName</key><string>Landline Menubar</string>
      <key>CFBundleDisplayName</key><string>Landline Menubar</string>
      <key>CFBundleExecutable</key><string>landline-menubar</string>
      <key>CFBundleIdentifier</key><string>dev.landline.menubar</string>
      <key>CFBundleIconFile</key><string>AppIcon</string>
      <key>CFBundlePackageType</key><string>APPL</string>
      <key>CFBundleShortVersionString</key><string>$version</string>
      <key>CFBundleVersion</key><string>$build</string>
      <key>LSMinimumSystemVersion</key><string>11.0</string>
      <key>LSUIElement</key><true/>
      <key>NSHighResolutionCapable</key><true/>
    </dict>
    </plist>
    PLIST
    plutil -lint "$app/Contents/Info.plist" > /dev/null
    codesign --force --deep -s - "$app"
    codesign --verify --deep --strict "$app"
    target="$folder/Landline Menubar.app"
    if [ -e "$target" ]; then mv "$target" "$work/previous.app"; fi
    mv "$app" "$target"
    echo "installed $target"

[private]
binaries folder:
    mkdir -p {{ folder }}
    stamp="$(tools/stamp/stamp.sh)"; commit="${stamp#commit=}"; commit="${commit%%$'\n'*}"; date="${stamp##*date=}"; \
      flags="-X main.version=$(tr -d '[:space:]' < VERSION)"; \
      if [ -n "$commit" ]; then flags="$flags -X main.commit=$commit"; fi; \
      if [ -n "$date" ]; then flags="$flags -X main.date=$date"; fi; \
      out="$(cd {{ folder }} && pwd)"; unset GOROOT; export CGO_ENABLED=0; \
      go build -trimpath -ldflags "$flags" -o "$out/landline-switchboard" ./cmd/landline-switchboard; \
      go build -trimpath -ldflags "$flags" -o "$out/landline-mcp" ./cmd/landline-mcp; \
      (cd tui && go build -trimpath -ldflags "$flags" -o "$out/landline" .); \
      if [ "$(uname -s)" = Darwin ]; then (cd menubar && CGO_ENABLED=1 CC="${CC:-/usr/bin/clang}" go build -trimpath -ldflags "$flags" -o "$out/landline-menubar" .); fi

# Everything CI runs.
check: fmt-check lint-go lint-tui lint-menubar lint-deps lint-acceptance lint-records test test-tui test-menubar

fmt:
    mdformat --wrap 120 {{ md_paths }}

fmt-check:
    mdformat --check --wrap 120 {{ md_paths }}

lint-go:
    test -z "$(gofmt -l .)" && go vet ./...

lint-tui:
    cd tui && test -z "$(gofmt -l .)" && go vet ./...

# macOS only: the menubar module needs cgo and the macOS SDK, so other systems skip it.
lint-menubar:
    @if [ "$(uname -s)" = Darwin ]; then cd menubar && test -z "$(gofmt -l .)" && go vet ./...; else echo "skipping menubar: macOS only"; fi

# Imports against the rules in architecture.yaml.
lint-deps:
    python3 tools/dependency-check/check.py

lint-acceptance:
    python3 tools/acceptance-imports/check.py

# Decision records, contracts, approvals, and the architecture file, through the Threnos flake checks.
lint-records:
    nix flake check

test:
    go test -race -count=1 ./...

test-tui:
    cd tui && go test -race -count=1 ./...

test-menubar:
    @if [ "$(uname -s)" = Darwin ]; then cd menubar && go test -race -count=1 ./...; else echo "skipping menubar: macOS only"; fi

# Make the menu bar and agent state PNGs in menubar/internal/icon (the jack from assets/jack.svg).
menubar-icons:
    cd menubar && go run ./tools/icons ../assets/jack.svg internal/icon

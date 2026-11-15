# One command surface for the repo. Run `just` to list recipes.
set shell := ["bash", "-euo", "pipefail", "-c"]

import "tools/mutation/mutation.just"

md_paths := "docs contracts AGENTS.md README.md"

default:
    @just --list

# Build landline, landline-switchboard, and landline-mcp into ./bin.
build: (binaries "bin")

# Build and install the three binaries into ~/.local/bin, or into prefix/bin when one is given.
install prefix="~/.local": (binaries prefix + "/bin")
    @echo "installed landline, landline-switchboard, and landline-mcp in {{ prefix }}/bin"

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
      (cd tui && go build -trimpath -ldflags "$flags" -o "$out/landline" .)

# Everything CI runs.
check: fmt-check lint-go lint-tui lint-deps lint-acceptance lint-records test test-tui

fmt:
    mdformat --wrap 120 {{ md_paths }}

fmt-check:
    mdformat --check --wrap 120 {{ md_paths }}

lint-go:
    test -z "$(gofmt -l .)" && go vet ./...

lint-tui:
    cd tui && test -z "$(gofmt -l .)" && go vet ./...

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

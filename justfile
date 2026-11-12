# One command surface for the repo. Run `just` to list recipes.
set shell := ["bash", "-euo", "pipefail", "-c"]

import "tools/mutation/mutation.just"

md_paths := "docs contracts AGENTS.md README.md"

default:
    @just --list

# Build the landline binary into ./bin.
build:
    unset GOROOT; CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$(tr -d '[:space:]' < VERSION)" -o bin/landline ./binary

# Build and install landline into ~/.local/bin, or into prefix/bin when one is given.
install prefix="~/.local":
    mkdir -p {{ prefix }}/bin
    unset GOROOT; CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$(tr -d '[:space:]' < VERSION)" -o {{ prefix }}/bin/landline ./binary
    @echo "installed {{ prefix }}/bin/landline"

# Build and install landline-tui into ~/.local/bin, or into prefix/bin when one is given.
install-tui prefix="~/.local":
    mkdir -p {{ prefix }}/bin
    out="$(cd {{ prefix }}/bin && pwd)"; cd tui && unset GOROOT; CGO_ENABLED=0 go build -trimpath -o "$out/landline-tui" .
    @echo "installed {{ prefix }}/bin/landline-tui"

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

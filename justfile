# One command surface for the repo. Run `just` to list recipes.
set shell := ["bash", "-euo", "pipefail", "-c"]

import "tools/mutation/mutation.just"

md_paths := "docs contracts AGENTS.md README.md"

default:
    @just --list

# Everything CI runs.
check: fmt-check lint-go lint-deps lint-acceptance test

fmt:
    mdformat --wrap 120 {{ md_paths }}

fmt-check:
    mdformat --check --wrap 120 {{ md_paths }}

lint-go:
    test -z "$(gofmt -l .)" && go vet ./...

# Imports against the rules in architecture.yaml.
lint-deps:
    python3 tools/dependency-check/check.py

lint-acceptance:
    python3 tools/acceptance-imports/check.py

test:
    go test -race -count=1 ./...

# Try Landline between two Claude Code sessions. See scripts/landline-dev.
dev-up:
    scripts/landline-dev up

dev-down:
    scripts/landline-dev down

dev-status:
    scripts/landline-dev status

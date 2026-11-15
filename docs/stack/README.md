# Stack

| Doc                              | Holds                                     |
| -------------------------------- | ----------------------------------------- |
| [code-layout.md](code-layout.md) | How the neutral terms map to Go folders   |
| [testing.md](testing.md)         | The test runner, the checks, and mutation |
| [build-stamp.md](build-stamp.md) | The version, commit, and date stamp       |
| [osv-scanner.md](osv-scanner.md) | Dependency vulnerability scans            |
| [repowise.md](repowise.md)       | Local repository context for agents       |

## Go modules

There are three binaries ([0110](../decisions/0110-three-binaries-landline-switchboard-and-landline-mcp.md)).
`landline-switchboard` and `landline-mcp` are entry points in the root module, `github.com/8thlight-daveloper/landline`,
under `cmd/`. The root module has no third-party Go dependencies. It imports only the standard library, and the flake
builds these two with `vendorHash = null`.

`landline` is built in `tui/`, its own module, `github.com/8thlight-daveloper/landline/tui`, with its own `go.mod` and
`go.sum` ([0090](../decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md)). The module
imports the root: its `go.mod` requires the root module and has a `replace` to the folder above it. Only `tui/main.go`
imports root packages, including `internal` ones, which Go allows because the import paths share the root. The screen
packages under `tui/internal/` import no root package. Third-party dependencies live only in the `tui/` module. The
flake builds `landline` from the whole source with `modRoot = "tui"` and `proxyVendor = true`, so its `vendorHash`
covers the third-party dependencies and changes when they change, and not when the root's code changes. Its direct
dependencies:

| Dependency                           | Version | Why                                                                             |
| ------------------------------------ | ------- | ------------------------------------------------------------------------------- |
| `github.com/charmbracelet/bubbletea` | v1.3.10 | The terminal program loop: keys, resize, and redraw                             |
| `github.com/charmbracelet/lipgloss`  | v1.1.0  | Styles for the screen's header, rows, and status line                           |
| `github.com/charmbracelet/x/ansi`    | v0.10.1 | Display width and truncation that count escape sequences and wide characters    |
| `github.com/mattn/go-isatty`         | v0.0.20 | Tells whether standard input and output are a terminal, before the screen opens |

Their indirect dependencies are pinned in `tui/go.sum`. Change a version in `tui/go.mod`, run `go mod tidy` in `tui/`,
and set the flake's `vendorHash` for `landline` to the hash that `nix build .#landline` reports.

## Tooling

| Need                 | Used                                                                                                      |
| -------------------- | --------------------------------------------------------------------------------------------------------- |
| Root module checks   | `just lint-go` and `just test`, which cover the root module only                                          |
| `tui/` module checks | `just lint-tui` and `just test-tui`, run by `just check`: gofmt, `go vet`, and `go test -race`            |
| Build and install    | `just build` builds all three binaries into `bin/`, and `just install` installs them                      |
| Nix build            | `nix build .#landline`, `.#landline-switchboard`, `.#landline-mcp`, or `.#default`, a bundle of all three |

# Stack

| Doc                              | Holds                                     |
| -------------------------------- | ----------------------------------------- |
| [code-layout.md](code-layout.md) | How the neutral terms map to Go folders   |
| [testing.md](testing.md)         | The test runner, the checks, and mutation |
| [osv-scanner.md](osv-scanner.md) | Dependency vulnerability scans            |
| [repowise.md](repowise.md)       | Local repository context for agents       |

## Go modules

The root module, `github.com/8thlight-daveloper/landline`, has no third-party Go dependencies. It imports only the
standard library, and the flake builds it with `vendorHash = null`. `landline tui` runs the other module's binary and
imports none of it.

`tui/` is its own module, `github.com/8thlight-daveloper/landline/tui`, with its own `go.mod` and `go.sum`
([0090](../decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md)). The flake builds it as
`landline-tui` with its own `vendorHash`. Its direct dependencies:

| Dependency                           | Version | Why                                                                          |
| ------------------------------------ | ------- | ---------------------------------------------------------------------------- |
| `github.com/charmbracelet/bubbletea` | v1.3.10 | The terminal program loop: keys, resize, and redraw                          |
| `github.com/charmbracelet/lipgloss`  | v1.1.0  | Styles for the screen's header, rows, and status line                        |
| `github.com/charmbracelet/x/ansi`    | v0.10.1 | Display width and truncation that count escape sequences and wide characters |

Their indirect dependencies are pinned in `tui/go.sum`. Change a version in `tui/go.mod`, run `go mod tidy` in `tui/`,
and set the flake's `vendorHash` for `landline-tui` to the hash that `nix build .#landline-tui` reports.

## Tooling

| Need                 | Used                                                                                           |
| -------------------- | ---------------------------------------------------------------------------------------------- |
| Root module checks   | `just lint-go` and `just test`, which cover the root module only                               |
| `tui/` module checks | `just lint-tui` and `just test-tui`, run by `just check`: gofmt, `go vet`, and `go test -race` |
| Install `tui/`       | `just install-tui` builds `landline-tui` into `~/.local/bin`                                   |
| Nix build            | `nix build .#landline-tui`                                                                     |

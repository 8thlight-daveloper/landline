# Stack

| Doc                              | Holds                                     |
| -------------------------------- | ----------------------------------------- |
| [code-layout.md](code-layout.md) | How the neutral terms map to Go folders   |
| [testing.md](testing.md)         | The test runner, the checks, and mutation |
| [build-stamp.md](build-stamp.md) | The version, commit, and date stamp       |
| [osv-scanner.md](osv-scanner.md) | Dependency vulnerability scans            |
| [repowise.md](repowise.md)       | Local repository context for agents       |

## Go modules

There are three binaries ([0110](../decisions/0110-three-binaries-landline-switchboard-and-landline-mcp.md)), and a
fourth on macOS
([0140](../decisions/0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md)).
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

`landline-menubar` is built in `menubar/`, its own module, `github.com/8thlight-daveloper/landline/menubar`, on macOS
only. It needs cgo and the macOS SDK, so a person cannot cross-build it from Linux. Its `go.mod` requires the root
module with a `replace` to the folder above it, as `tui/` does. Only `menubar/main.go` imports a root package, for the
version line. The packages under `menubar/internal/` import none and keep their own copies of the switchboard's types.
The root module and `tui/` gain none of its dependencies. The flake builds it with `modRoot = "menubar"`,
`proxyVendor = true`, and its own `vendorHash`, for Darwin systems only. Its direct dependency:

| Dependency        | Version | Why                                                                                                                                                                                                      |
| ----------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `fyne.io/systray` | v1.12.2 | The status item. It builds here with cgo, takes template images, nests submenus, enables and disables items, and rebuilds the menu with `ResetMenu`. It has no web view and sets no Dock icon on its own |

Its indirect dependencies, `github.com/godbus/dbus/v5` and `golang.org/x/sys`, are pinned in `menubar/go.sum`. The
status item calls `SetTemplateIcon` with the 44 pixel image, because the library sizes every image to 16 points and
offers no second resolution. The app sets the accessory activation policy through its own few lines of Objective-C, so
it has no Dock icon and needs no app bundle. The same lines show the standard About panel.

### Menu bar images

The images are made from `assets/jack.svg` by `menubar/tools/icons`, a Go program that reads the drawing's viewBox and
path and fills the path at pixel centres, with no other tool. The drawing is a grid of 11 units with the jack in the
middle, so each of its edges falls on a pixel boundary at every size and no pixel is blurred. Run `just menubar-icons`
and commit the result. A second run gives the same bytes, and the program's test checks that. The files are in
`menubar/internal/icon/`:

| File                   | Pixels | Ink                               | Used for                                        |
| ---------------------- | ------ | --------------------------------- | ----------------------------------------------- |
| `jack-template-22.png` | 22     | Black on transparent              | The menu bar template image, at 1x              |
| `jack-template-44.png` | 44     | Black on transparent              | The menu bar template image the app loads       |
| `jack-dimmed-22.png`   | 22     | Black at 40% alpha, transparent   | The menu bar image when the switchboard is down |
| `jack-dimmed-44.png`   | 44     | Black at 40% alpha, transparent   | The dimmed image the app loads                  |
| `jack-about-440.png`   | 440    | Brand blue `#0071CE`, transparent | The About panel icon, 40 times the grid         |

### App bundle

`just install-app [folder]` builds `Landline Menubar.app` and installs it into `folder`, `~/Applications` by default.
`just install` calls it on macOS, and it prints a skip message elsewhere. The recipe builds the stamped binary itself
and makes everything in a hidden temporary folder inside the target folder, so the final step is a `mv` on one
filesystem. It checks the plist with `plutil -lint`, signs with `codesign --force --deep -s -`, runs
`codesign --verify --deep --strict`, moves any existing bundle aside, and moves the new one in. It never writes outside
the folder, `~/.local/bin`, and temp files. The bundle holds:

| Path                              | Content                                                                                                                                                                        |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `Contents/MacOS/landline-menubar` | The same binary and stamp as `just build`                                                                                                                                      |
| `Contents/Info.plist`             | `Landline Menubar` name, `dev.landline.menubar` identifier, `VERSION` as the short version, the short commit as the build (the version when there is no commit), `LSUIElement` |
| `Contents/Resources/AppIcon.icns` | The jack in brand blue `#0071CE` on transparent                                                                                                                                |

The icon is not committed. Each run of the recipe runs `go run ./tools/icons -iconset ../assets/jack.svg <folder>` in
`menubar/`, which writes the ten iconset PNGs from the same drawing and the same renderer as the menu bar images, and
then `iconutil -c icns`. The program's output is the same on every run. The bundle's minimum system version is 11.0, the
first macOS for Apple silicon. A bundle started from Spotlight has no shell environment, so `landline` must be on the
login shell's path.

### Moving the repository

The About panel links to the project's GitHub page. The address is the one constant `Repository` in
`menubar/internal/about/repository.go`. When the project moves to the 8th Light organization, change that line and the
links in `README.md`.

## Tooling

| Need                 | Used                                                                                                                                                                   |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Root module checks   | `just lint-go` and `just test`, which cover the root module only                                                                                                       |
| `tui/` module checks | `just lint-tui` and `just test-tui`, run by `just check`: gofmt, `go vet`, and `go test -race`                                                                         |
| `menubar/` checks    | `just lint-menubar` and `just test-menubar`, run by `just check` on macOS and skipped elsewhere                                                                        |
| Menu bar images      | `just menubar-icons` rewrites the PNGs in `menubar/internal/icon/` from `assets/jack.svg`                                                                              |
| Build and install    | `just build` builds the binaries into `bin/`, and `just install` installs them. `landline-menubar` only on macOS                                                       |
| Nix build            | `nix build .#landline`, `.#landline-switchboard`, `.#landline-mcp`, or `.#default`, a bundle of all. `.#landline-menubar` and its place in `.#default` are Darwin only |

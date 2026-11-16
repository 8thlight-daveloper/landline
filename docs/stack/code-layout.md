# Code layout

How the neutral terms in [architecture.yaml](../../architecture.yaml) map to Go folders.

| Term           | In Go                                                                                                     |
| -------------- | --------------------------------------------------------------------------------------------------------- |
| Component      | One package under `internal/<noun>/<role>`. The directory carries the noun, the package the role          |
| Domain model   | `internal/model`                                                                                          |
| TUI            | `tui/`, its own Go module that builds `landline`, outside `components_root`. Component `agent/viewer`     |
| Menu bar item  | `menubar/`, its own Go module that builds `landline-menubar`, macOS only. Component `menubar/viewer`      |
| Entry points   | `cmd/landline-switchboard` and `cmd/landline-mcp`, thin `main` packages, and `tui/main.go` for `landline` |
| Interface file | `api.go` in the contract package, with the interface and its data types                                   |
| Contract tests | `<role>_test.go` in the contract package                                                                  |
| Wiring test    | `wire_test.go`, the only file in a contract package that imports its implementation                       |
| Fixtures       | `testdata/` folders, with a `cases.json` manifest                                                         |

```text
internal/
  model/                 shared types, imports nothing from the project
  <noun>/<role>/         one component per package
contracts/
  <noun>/<role>/         api.go, contract.md, <role>_test.go, wire_test.go, testdata/
acceptance/              scenarios as Go code, and their runner
tui/                     the landline module: main.go and internal/ packages, with its own go.mod
cmd/                     thin mains: landline-switchboard and landline-mcp
tools/mutation/accepted/<noun>/<role>.txt
```

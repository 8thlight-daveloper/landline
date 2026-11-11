# Go dependency check

Checks a Go project's imports against the dependency rules in `architecture.yaml`. It enforces the position order,
`depends_on`, the functional core's banned categories, and the naming rule, as
[ADR 0100](../../../docs/decisions/0100-dependency-rules-are-checked-from-architecture-yaml.md) and
[structure](../../../process/structure.md) describe.

## How it works

`check.py` runs `go list -e -json` over `components_root` and the model, which resolves imports exactly. It reads each
package's non-test imports and reports every violation with the folder and the import that proves it.

| Rule                          | Reported when                                                                                                         |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `depends_on`                  | A component imports a component that its `depends_on` does not list                                                   |
| Position order                | A component imports one at the same or a later position                                                               |
| Ownership                     | A package under `components_root` belongs to no component, or a component imports one that does not                   |
| Edges to the rest of the repo | A component or the model imports a package outside the components and the model                                       |
| The model                     | The model imports a component                                                                                         |
| `banned_in_core`              | A core component or the model imports a library in a listed category                                                  |
| Clock                         | A core component uses `time.Now`, `Since`, `Until`, `Sleep`, `After`, `Tick`, `NewTimer`, `NewTicker`, or `AfterFunc` |
| Naming                        | A component name does not follow `naming.component_path` and is not in `naming.exceptions`                            |

A package inside a component's folder belongs to that component, and the longest matching name wins. A **core**
component is any component not listed under `drivers`. Drivers are exempt from `banned_in_core` and still follow
`depends_on` and position order. The entry point is outside `components_root` and is not checked. The model is always an
allowed import, and it is core.

## Categories

The standard library part of each category is built in.

| Category      | Built in                                                                      |
| ------------- | ----------------------------------------------------------------------------- |
| `filesystem`  | `os`, `io/fs`, `io/ioutil`, `path/filepath`                                   |
| `network`     | `net`, `net/http` and below, `net/rpc` and below, `net/smtp`, `net/textproto` |
| `process`     | `os/exec`, `os/signal`, `syscall`                                             |
| `randomness`  | `math/rand`, `math/rand/v2`, `crypto/rand`                                    |
| `environment` | `os`, `os/user`, `runtime/debug`                                              |
| `terminal`    | None. The project lists them                                                  |
| `clock`       | Calls, not imports. `time` itself stays importable for its types              |

`net/url`, `net/netip`, and `net/mail` parse text and are allowed. A project adds third-party libraries to a category
under `libraries` in `architecture.yaml`. An entry also covers the packages below it:

```yaml
libraries:
  terminal:
    - charm.land/bubbletea/v2
    - github.com/charmbracelet/x/ansi
```

## Clock calls

An import ban on `time` would also ban `time.Duration`, which the model needs. `timecalls.go` parses each core Go file
and reports a use of one of the functions above through any import name. A dot import of `time` is reported, because its
calls cannot be told from others. A local variable that shadows `time` is not.

## Flat names

A project that has not decomposed a component yet lists its name under `naming.exceptions`. The check fails on a flat
name that is not listed, and on a listed name that follows the rule or is not a component. It prints how many exceptions
remain on each run. The list is protected like the rest of `architecture.yaml`.

```yaml
naming:
  component_path: "<noun>/<role>"
  exceptions: [session, terminal]
```

## Use it

1. Copy this folder to `tools/dependency-check/` in the project.
1. Put `go` and Python with PyYAML in the dev shell. The Threnos dev shell has PyYAML.
1. Run it from the `check` recipe: `python3 tools/dependency-check/check.py`. A flake check cannot run it, because the
   Nix sandbox cannot fetch Go modules. CI reaches it through `nix develop --command just check`.

## Limits

It reads imports and clock functions, not what a core component does with a value it was given. The functional core's
other checks stay in the contract tests. Build constraints apply as `go list` applies them, so a file excluded for the
current platform is not checked. Test files are not checked, and neither are the imports of `acceptance/`, which the
[acceptance imports check](../acceptance-imports/README.md) covers.

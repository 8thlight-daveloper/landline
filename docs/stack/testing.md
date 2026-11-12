# Testing

| Need               | Used                                                        | Notes                                                          |
| ------------------ | ----------------------------------------------------------- | -------------------------------------------------------------- |
| Test runner        | `go test -race -count=1 ./...`                              | Run a build's tests with `-count=3` too, to catch hidden state |
| Dependency rules   | `tools/dependency-check/check.py`                           | Imports and clock calls against `architecture.yaml`            |
| Acceptance imports | `tools/acceptance-imports/check.py`                         | Scenarios import no implementation package                     |
| Mutation           | gomu, through `tools/mutation/` (`just mutate <component>`) | Runs against a core component's contract                       |
| Decision records   | `nix flake check` (`threnos-records`)                       | Run by `just check`. Front matter, links, and approvals        |
| Markdown format    | `mdformat --wrap 120`                                       | Run by `just check`                                            |

## Time and ports in acceptance

A scenario drives the built binary, so it cannot move a clock. The hub takes `--keepalive`, `--delivery-timeout`, and
`--retention` as Go durations. A scenario starts the hub with values of a few hundred milliseconds and waits for the
real interval to pass. The hub's own tests move the injected clock `Config.Now` instead.

A scenario starts every hub on `--addr 127.0.0.1:0` or a free port it picked. The one check of the default address sets
`LANDLINE_PORT` to a free port, so it runs while a person's own hub holds 8787.

# Testing

| Need               | Used                                                        | Notes                                                          |
| ------------------ | ----------------------------------------------------------- | -------------------------------------------------------------- |
| Test runner        | `go test -race -count=1 ./...`                              | Run a build's tests with `-count=3` too, to catch hidden state |
| Dependency rules   | `tools/dependency-check/check.py`                           | Imports and clock calls against `architecture.yaml`            |
| Acceptance imports | `tools/acceptance-imports/check.py`                         | Scenarios import no implementation package                     |
| Mutation           | gomu, through `tools/mutation/` (`just mutate <component>`) | Runs against a core component's contract                       |
| Decision records   | `nix flake check` (`threnos-records`)                       | Front matter, links, and `architecture.yaml` ids               |
| Markdown format    | `mdformat --wrap 120`                                       | Run by `just check`                                            |

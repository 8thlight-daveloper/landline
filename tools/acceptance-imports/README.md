# Go acceptance imports check

Checks that a Go project's acceptance scenarios import no implementation package. It enforces `acceptance.may_import` in
`architecture.yaml`, as [ADR 0060](../../../docs/decisions/0060-acceptance-scenarios-as-data-or-code.md) and
[acceptance scenarios](../../../process/acceptance.md) describe. It applies to scenarios written as data, whose runner
is Go code, and to scenarios written as code.

## How it works

`check.py` runs `go list -e -json -test ./acceptance/...`, which resolves imports exactly and includes test files. It
collects each package's `Imports`, `TestImports`, and `XTestImports`. An import is allowed when it is:

- in the standard library,
- under `acceptance/` in the project's own module, or
- listed in `acceptance.may_import`. An entry also allows the packages below it.

Anything else is a violation, and the project's own implementation packages are the case it exists to catch. The check
prints the package folder and the import, and exits 1. A project that wants a third-party test library lists its import
path.

```yaml
acceptance:
  scenarios: code
  may_import: [github.com/example/terminal-emulator]
```

## Use it

1. Copy this folder to `tools/acceptance-imports/` in the project.
1. Put `go` and Python with PyYAML in the dev shell. The Threnos dev shell has PyYAML.
1. Run it from the `check` recipe: `python3 tools/acceptance-imports/check.py`.

## Limits

It reads imports, not what a scenario does with a built program, so it cannot tell that a scenario started the binary.
It does not check imports outside `acceptance/`. Build constraints apply as `go list` applies them, so a file excluded
for the current platform is not checked.

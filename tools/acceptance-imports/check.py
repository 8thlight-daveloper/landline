"""Check that a Go project's acceptance scenarios import no implementation package.

Usage: check.py [project root]

Reads `acceptance.may_import` from architecture.yaml and the imports of every package under acceptance/, test files
included, from `go list`. An import is allowed when it is in the standard library, in a package under acceptance/, or
listed in `acceptance.may_import`. Anything else is a violation, and the project's own implementation packages are
the case it exists to catch. Exits 1 on a violation.

An entry in `may_import` is an import path. It also allows the packages below it, so `github.com/hinshun/vt10x` allows
`github.com/hinshun/vt10x/internal/x`.
"""
import json
import subprocess
import sys
from pathlib import Path

import yaml

ACCEPTANCE = "acceptance"


def stdlib(path):
    return "." not in path.split("/")[0]


def listed(path, may_import):
    return any(path == entry or path.startswith(entry.rstrip("/") + "/") for entry in may_import)


def violations(packages, module, may_import):
    """Return (package dir, import path) for each import that is not allowed.

    packages maps a package directory to the set of its import paths.
    """
    found = []
    own = module + "/"
    inside = own + ACCEPTANCE
    for directory, imports in sorted(packages.items()):
        for path in sorted(imports):
            if path == "C" or path == "unsafe":
                continue
            under_acceptance = path == inside or path.startswith(inside + "/")
            is_own = path == module or path.startswith(own)
            if under_acceptance or listed(path, may_import):
                continue
            if is_own or not stdlib(path):
                found.append((directory, path))
    return found


def go_packages(root):
    """Return {package directory: imports} for the real packages under acceptance/, tests included."""
    run = subprocess.run(
        ["go", "list", "-e", "-json", "-test", f"./{ACCEPTANCE}/..."],
        cwd=root, capture_output=True, text=True,
    )
    if run.returncode != 0:
        raise SystemExit(f"go list failed:\n{run.stderr}")
    packages, decoder, text, position = {}, json.JSONDecoder(), run.stdout, 0
    while True:
        text_left = text[position:].lstrip()
        if not text_left:
            break
        position = len(text) - len(text_left)
        package, end = decoder.raw_decode(text, position)
        position = end
        if package.get("ForTest") or package["ImportPath"].endswith(".test"):
            continue
        imports = set(package.get("Imports") or []) | set(package.get("TestImports") or []) | set(package.get("XTestImports") or [])
        directory = Path(package["Dir"]).relative_to(root.resolve()).as_posix()
        packages.setdefault(directory, set()).update(imports)
    return packages


def main(args):
    root = Path(args[0] if args else ".").resolve()
    architecture = yaml.safe_load((root / "architecture.yaml").read_text()) or {}
    may_import = (architecture.get("acceptance") or {}).get("may_import") or []
    module = subprocess.run(["go", "list", "-m"], cwd=root, capture_output=True, text=True, check=True).stdout.strip()
    packages = go_packages(root)
    found = violations(packages, module, may_import)
    for directory, path in found:
        print(f"{directory}: imports {path}, which is not in the standard library, under {ACCEPTANCE}/, or in acceptance.may_import", file=sys.stderr)
    if found:
        return 1
    print(f"{ACCEPTANCE} imports are consistent")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))

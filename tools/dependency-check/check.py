"""Check a Go project's imports against the dependency rules in architecture.yaml.

Usage: check.py [project root]

Reads components_root, model, components (name, position, depends_on), drivers, banned_in_core, libraries, and naming
from architecture.yaml. Reads each package's non-test imports under components_root, and under the model, from
`go list`. Reports each violation with the import that proves it:

- a component imports a component that is not in its depends_on, or is not at an earlier position;
- a component or the model imports a package that no component owns, or that sits outside the components and the model;
- the model imports a component;
- a core component, which is any component not listed under drivers, or the model, imports a library in a category listed
  under banned_in_core, or uses a function of `time` that reads the clock (when `clock` is listed);
- a package under components_root belongs to no component;
- a component name does not follow naming.component_path and is not listed under naming.exceptions.

Drivers are exempt from banned_in_core and still follow depends_on and position. The entry point is outside
components_root and is not checked. Test files are not checked. Exits 1 on a violation.
"""
import json
import re
import subprocess
import sys
from pathlib import Path

import yaml

# An entry is an exact import path, or a path ending in /... for the package and everything below it.
BUILTIN_CATEGORIES = {
    "filesystem": ["os", "io/fs", "io/ioutil", "path/filepath"],
    "network": ["net", "net/http/...", "net/rpc/...", "net/smtp", "net/textproto"],
    "process": ["os/exec", "os/signal", "syscall"],
    "randomness": ["math/rand", "math/rand/v2", "crypto/rand"],
    "environment": ["os", "os/user", "runtime/debug"],
    "terminal": [],
    "clock": [],
}
DEFAULT_COMPONENT_PATH = "<noun>/<role>"
DEFAULT_SEGMENT = "^[a-z][a-z0-9]*$"
HELPER = Path(__file__).with_name("timecalls.go")


def matches(path, entries):
    for entry in entries:
        if entry.endswith("/..."):
            base = entry[:-4]
            if path == base or path.startswith(base + "/"):
                return True
        elif path == entry:
            return True
    return False


def categories_of(path, libraries):
    """Return the categories whose libraries include this import path."""
    return [
        category
        for category, builtin in BUILTIN_CATEGORIES.items()
        if matches(path, builtin + [entry.rstrip("/") + "/..." for entry in libraries.get(category) or []])
    ]


class Rules:
    def __init__(self, architecture):
        self.root = (architecture.get("components_root") or "").strip("/")
        self.model = (architecture.get("model") or "").strip("/") or None
        self.components = {each["name"]: each for each in architecture.get("components") or []}
        self.drivers = set(architecture.get("drivers") or [])
        self.banned = set(architecture.get("banned_in_core") or [])
        self.libraries = architecture.get("libraries") or {}
        naming = architecture.get("naming") or {}
        self.component_path = naming.get("component_path") or DEFAULT_COMPONENT_PATH
        self.segment = re.compile(naming.get("segment_pattern") or DEFAULT_SEGMENT)
        self.exceptions = list(naming.get("exceptions") or [])

    def position(self, name):
        return self.components[name]["position"]

    def depends_on(self, name):
        return self.components[name].get("depends_on") or []

    def owner(self, directory):
        """Return the component that owns a package folder, or None."""
        if directory != self.root and not directory.startswith(self.root + "/"):
            return None
        inside = directory[len(self.root) + 1:]
        owners = [name for name in self.components if inside == name or inside.startswith(name + "/")]
        return max(owners, key=len) if owners else None

    def in_model(self, directory):
        return self.model is not None and (directory == self.model or directory.startswith(self.model + "/"))

    def under_root(self, directory):
        return directory == self.root or directory.startswith(self.root + "/")


def rule_problems(rules):
    """Return the problems in architecture.yaml itself."""
    problems = []
    if not rules.root:
        problems.append("architecture.yaml: components_root is not set")
    for name in sorted(rules.drivers - set(rules.components)):
        problems.append(f"architecture.yaml: driver {name} is not a component")
    for name in rules.components:
        for needed in rules.depends_on(name):
            if needed not in rules.components:
                problems.append(f"architecture.yaml: {name} depends on {needed}, which is not a component")
            elif rules.position(needed) >= rules.position(name):
                problems.append(
                    f"architecture.yaml: {name} (position {rules.position(name)}) depends on {needed} "
                    f"(position {rules.position(needed)}), which is not earlier"
                )
    segments = rules.component_path.count("/") + 1
    for name in rules.components:
        parts = name.split("/")
        follows = len(parts) == segments and all(rules.segment.match(part) for part in parts)
        if not follows and name not in rules.exceptions:
            problems.append(
                f"architecture.yaml: component {name} does not follow naming.component_path {rules.component_path}. "
                "Decompose it, or list it under naming.exceptions"
            )
        if follows and name in rules.exceptions:
            problems.append(f"architecture.yaml: {name} is listed under naming.exceptions but follows the naming rule")
    for name in rules.exceptions:
        if name not in rules.components:
            problems.append(f"architecture.yaml: naming.exceptions lists {name}, which is not a component")
    return problems


def violations(packages, module, rules):
    """Return (package folder, message) for each broken rule.

    packages maps a package folder, relative to the project root, to its non-test import paths.
    """
    found = []
    own = module + "/"
    for directory, imports in sorted(packages.items()):
        model = rules.in_model(directory)
        component = None if model else rules.owner(directory)
        if not model and component is None:
            if rules.under_root(directory):
                found.append((directory, f"belongs to no component. List it in architecture.yaml, or move it out of {rules.root}/"))
            continue
        core = model or component not in rules.drivers
        for path in sorted(imports):
            if path.startswith(own):
                found += own_import_violations(directory, path[len(own):], component, model, rules)
            elif core:
                banned = [each for each in categories_of(path, rules.libraries) if each in rules.banned]
                if banned:
                    found.append((directory, f"imports {path}, which is banned in core ({', '.join(banned)})"))
    return found


def own_import_violations(directory, target, component, model, rules):
    if rules.in_model(target):
        return []
    owner = rules.owner(target)
    if model:
        if owner is not None:
            return [(directory, f"is the model and imports component {owner} ({target})")]
        return [(directory, f"is the model and imports {target}, which is outside the components and the model")]
    if owner is None:
        if rules.under_root(target):
            return [(directory, f"imports {target}, which belongs to no component")]
        return [(directory, f"imports {target}, which is outside the components and the model")]
    if owner == component:
        return []
    found = []
    if owner not in rules.depends_on(component):
        found.append((directory, f"imports component {owner} ({target}), which is not in depends_on of {component}"))
    if rules.position(owner) >= rules.position(component):
        found.append((
            directory,
            f"imports component {owner} ({target}) at position {rules.position(owner)}, "
            f"not earlier than {component} at position {rules.position(component)}",
        ))
    return found


def clock_violations(files_by_folder, rules, root):
    """Return (folder, message) for each use of a clock function in a core package."""
    core = {
        folder: files
        for folder, files in files_by_folder.items()
        if rules.in_model(folder) or (rules.owner(folder) is not None and rules.owner(folder) not in rules.drivers)
    }
    names = [str(Path(root) / folder / name) for folder, files in core.items() for name in files]
    if not names:
        return []
    run = subprocess.run(["go", "run", str(HELPER)], cwd=root, input="\n".join(names), capture_output=True, text=True)
    if run.returncode != 0:
        raise SystemExit(f"the clock scan failed:\n{run.stderr}")
    found = []
    for line in run.stdout.splitlines():
        path, number, name = line.rsplit(":", 2)
        relative = Path(path).relative_to(Path(root).resolve())
        what = "dot-imports time, which hides its clock calls" if name == "." else f"uses time.{name}, which reads the clock"
        found.append((relative.parent.as_posix(), f"{relative.name}:{number} {what}, banned in core (clock)"))
    return found


def go_packages(root, rules):
    """Return ({folder: imports}, {folder: file names}, problems) for the packages under components_root and the model."""
    patterns = [f"./{rules.root}/..."] + ([f"./{rules.model}/..."] if rules.model else [])
    run = subprocess.run(["go", "list", "-e", "-json", *patterns], cwd=root, capture_output=True, text=True)
    if run.returncode != 0:
        raise SystemExit(f"go list failed:\n{run.stderr}")
    imports, files, problems = {}, {}, []
    decoder, text, position = json.JSONDecoder(), run.stdout, 0
    while True:
        left = text[position:].lstrip()
        if not left:
            break
        position = len(text) - len(left)
        package, position = decoder.raw_decode(text, position)
        if "Dir" not in package:
            continue
        folder = Path(package["Dir"]).relative_to(Path(root).resolve()).as_posix()
        if package.get("Error"):
            problems.append((folder, f"go list: {package['Error'].get('Err', 'error')}"))
        imports.setdefault(folder, set()).update(package.get("Imports") or [])
        files.setdefault(folder, []).extend(package.get("GoFiles") or [])
    return imports, files, problems


def main(args):
    root = Path(args[0] if args else ".").resolve()
    architecture = yaml.safe_load((root / "architecture.yaml").read_text()) or {}
    rules = Rules(architecture)
    problems = [f"{each}" for each in rule_problems(rules)]
    if problems:
        print("\n".join(problems), file=sys.stderr)
        return 1
    module = subprocess.run(["go", "list", "-m"], cwd=root, capture_output=True, text=True, check=True).stdout.strip()
    imports, files, listed_problems = go_packages(root, rules)
    found = listed_problems + violations(imports, module, rules)
    if "clock" in rules.banned:
        found += clock_violations(files, rules, root)
    for folder, message in sorted(set(found)):
        print(f"{folder}: {message}", file=sys.stderr)
    if rules.exceptions:
        print(f"naming exceptions in use: {len(rules.exceptions)}", file=sys.stderr)
    if found:
        return 1
    print("dependencies are consistent")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))

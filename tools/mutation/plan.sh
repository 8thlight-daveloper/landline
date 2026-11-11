#!/usr/bin/env bash
set -eu
cd "$(git rev-parse --show-toplevel)"

base="${1:-}"
components_dir="${COMPONENTS_DIR:-internal}"
contracts_dir="${CONTRACTS_DIR:-contracts}"
model_dir="${MODEL_DIR:-$components_dir/model}"
read -ra shared_paths <<< "${SHARED_PATHS:-$model_dir go.mod go.sum tools/mutation flake.nix flake.lock justfile .github/workflows/mutate.yml}"

components() {
    for folder in "$components_dir"/*/ "$components_dir"/*/*/; do
        name="${folder#"$components_dir"/}"
        name="${name%/}"
        [ -d "$contracts_dir/$name" ] || continue
        ls "$folder"*.go > /dev/null 2>&1 || continue
        echo "$name"
    done
}

everything() {
    components | jq -R . | jq -sc .
}

if [ -z "$base" ] || ! git cat-file -e "$base^{commit}" 2> /dev/null; then
    everything
    exit 0
fi

shared="$(git diff --name-only "$base" HEAD -- "${shared_paths[@]}")"
if [ -n "$shared" ]; then
    everything
    exit 0
fi

components | while read -r name; do
    if [ -n "$(git diff --name-only "$base" HEAD -- "$components_dir/$name" "$contracts_dir/$name")" ]; then
        echo "$name"
    fi
done | jq -R . | jq -sc .

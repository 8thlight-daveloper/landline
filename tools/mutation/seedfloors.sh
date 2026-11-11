#!/usr/bin/env bash
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: seedfloors.sh <results-dir> <margin>" >&2
    exit 2
fi

results="$(realpath "$1")"
margin="$2"
cd "$(dirname "$0")"
mkdir -p floor

find "$results" -name result.json -exec cat {} + | jq -r 'select(.score != null) | [.component, .score] | @tsv' | while IFS=$'\t' read -r component score; do
    target="floor/$component.txt"
    if [ -f "$target" ]; then
        echo "skip $component: floor exists" >&2
        continue
    fi
    mkdir -p "$(dirname "$target")"
    awk -v s="$score" -v m="$margin" 'BEGIN { f = s - m; if (f < 0) f = 0; printf "%.4f\n", int(f * 10000 + 0.000001) / 10000 }' > "$target"
    echo "$component $(cat "$target")"
done

#!/usr/bin/env bash
set -eu

results="$(realpath "$1")"
cd "$(dirname "$0")"

threshold="${THRESHOLD_MINUTES:-120}"
floors="${FLOOR_DIR:-$PWD/floor}"
bar_color="#4c78a8"
threshold_color="#f59e0b"

rows="$(find "$results" -name result.json -exec cat {} + | jq -s 'sort_by(-.seconds)')"
count="$(echo "$rows" | jq 'length')"
if [ "$count" = "0" ]; then
    echo "No result.json under $results" >&2
    exit 1
fi

echo "## Mutation run"
echo
echo "$count components. Time threshold: $threshold min."
echo
echo "| Component | Mutation time | Score | Floor | Against floor | Survivors | Accepted | Timed out |"
echo "| -- | -- | -- | -- | -- | -- | -- | -- |"
echo "$rows" | jq -r '.[] | [.component, .seconds, (.score // "none"), (.survivors // "none"), (.accepted // "none"), (.timedOut // "none")] | @tsv' | while IFS=$'\t' read -r component seconds score survivors accepted timedout; do
    minutes="$(awk -v s="$seconds" 'BEGIN { printf "%.1f", s / 60 }')"
    floor="none"
    verdict="n/a"
    if [ -f "$floors/$component.txt" ]; then
        floor="$(tr -d '[:space:]' < "$floors/$component.txt")"
        if [ "$score" = "none" ]; then
            verdict="NO SCORE"
        else
            verdict="$(awk -v s="$score" -v f="$floor" 'BEGIN { d = s - f; printf "%+.4f%s", d, (d < 0 ? "  BELOW" : "") }')"
        fi
    fi
    echo "| $component | \`$(./bar.sh "$minutes" "$threshold")\` | $score | $floor | $verdict | $survivors | $accepted | $timedout |"
done
echo
echo '```mermaid'
echo "$rows" | jq -r --argjson threshold "$threshold" --arg palette "$bar_color, $threshold_color" '
    (map((.seconds / 60 * 10 | round) / 10)) as $minutes
    | (([$threshold] + $minutes | max) * 1.1 | ceil) as $ceiling
    | ([300, length * 28 + 140] | max) as $height
    | "---",
      "config:",
      "  themeVariables:",
      "    xyChart:",
      "      plotColorPalette: \"\($palette)\"",
      "  xyChart:",
      "    width: 900",
      "    height: \($height)",
      "---",
      "xychart-beta horizontal",
      "    title \"Mutation minutes, \(length) components\"",
      "    x-axis [\(map("\"" + .component + "\"") | join(", "))]",
      "    y-axis \"minutes\" 0 --> \($ceiling)",
      "    bar [\($minutes | join(", "))]",
      "    line [\(map($threshold) | join(", "))]"'
echo '```'

below=0
while IFS=$'\t' read -r component score; do
    [ -f "$floors/$component.txt" ] || continue
    floor="$(tr -d '[:space:]' < "$floors/$component.txt")"
    if [ "$score" = "none" ] || awk -v s="$score" -v f="$floor" 'BEGIN { exit !(s < f) }'; then
        echo "below floor: $component (score $score, floor $floor)" >&2
        below=$((below + 1))
    fi
done < <(echo "$rows" | jq -r '.[] | [.component, (.score // "none")] | @tsv')
[ "${FAIL_BELOW_FLOOR:-0}" != "1" ] || [ "$below" = "0" ]

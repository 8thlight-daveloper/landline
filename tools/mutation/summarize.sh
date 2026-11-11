#!/usr/bin/env bash
set -eu

component="$1"
log="$(realpath "$2")"
seconds="$3"
start="$PWD"
cd "$(dirname "$0")"
threshold="${THRESHOLD_MINUTES:-120}"

minutes="$(awk -v s="$seconds" 'BEGIN { printf "%.1f", s / 60 }')"
score="$(grep -oE '^The mutation score is [0-9.]+' "$log" | grep -oE '[0-9.]+$' || true)"

counts="$(grep -E '^The mutation score' "$log" | sed -E 's/.*\(([0-9]+) passed, ([0-9]+) failed, ([0-9]+) accepted, ([0-9]+) not viable, ([0-9]+) timed out, ([0-9]+) errors, total is ([0-9]+)\).*/\1 \2 \3 \4 \5 \6 \7/' || true)"
case "$counts" in
    *[!0-9\ ]* | "") counts="" ;;
esac
read -r killed survivors accepted notviable timedout errors total <<< "${counts:-null null null null null null null}"
jq -n --arg component "$component" --argjson seconds "$seconds" --arg score "$score" \
    --argjson killed "$killed" --argjson survivors "$survivors" --argjson accepted "$accepted" \
    --argjson notViable "$notviable" --argjson timedOut "$timedout" --argjson errors "$errors" --argjson total "$total" \
    '{component: $component, seconds: $seconds, score: (if $score == "" then null else ($score | tonumber) end),
      killed: $killed, survivors: $survivors, accepted: $accepted, notViable: $notViable,
      timedOut: $timedOut, errors: $errors, total: $total}' > "$start/result.json"

if [ -n "$score" ]; then
    headline="score $score, $survivors survivors, $minutes min"
else
    headline="no score, $minutes min"
fi

echo "<details>"
echo "<summary><strong>$component</strong>: $headline</summary>"
echo
if [ -n "$score" ]; then
    grep -E '^The mutation score' "$log"
else
    echo "No score. The run did not finish."
fi
echo
echo '```'
echo "mutation time  $(./bar.sh "$minutes" "$threshold")"
echo '```'
echo
echo '```'
sed -n '/^Processing /,$p' "$log" | grep -vE '^(The mutation score|Report written)' | head -c 900000
echo '```'
echo
echo "</details>"

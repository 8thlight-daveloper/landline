#!/usr/bin/env bash
set -eu

width=20
minutes="$1"
threshold="$2"

filled="$(awk -v m="$minutes" -v t="$threshold" -v w="$width" 'BEGIN { f = int(m * w / t + 0.5); if (f > w) f = w; print f }')"
bar=""
for ((i = 0; i < width; i++)); do
    if [ "$i" -lt "$filled" ]; then bar="${bar}▓"; else bar="${bar}░"; fi
done

over=""
if awk -v m="$minutes" -v t="$threshold" 'BEGIN { exit !(m > t) }'; then
    over="  OVER"
fi
printf '%s  %s of %s min%s\n' "$bar" "$minutes" "$threshold" "$over"

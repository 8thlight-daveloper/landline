#!/bin/sh
commit=""
date=""
if command -v git >/dev/null 2>&1; then
	hash=$(git rev-parse --verify -q HEAD 2>/dev/null) || hash=""
	case "$hash" in
	*[!0-9a-f]*) hash="" ;;
	esac
	if [ "${#hash}" -eq 40 ]; then
		git update-index -q --refresh >/dev/null 2>&1
		if git diff-index --quiet HEAD -- >/dev/null 2>&1; then
			stamp=$(TZ=UTC git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ HEAD 2>/dev/null) || stamp=""
			commit="$hash"
			date="$stamp"
		else
			commit="$hash-dirty"
		fi
	fi
fi
printf 'commit=%s\ndate=%s\n' "$commit" "$date"
exit 0

#!/usr/bin/env bash
# Builds the complete programs (code blocks with "package main") of the skill for AI agents against the code of the repository,
# so the skill does not describe the API that does not exist.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
skill=${1:-$root/skills/tinyconf/SKILL.md}
# SKILL_WORKDIR keeps the built programs to run them by hand
if [ -n "${SKILL_WORKDIR:-}" ]; then
	work=$SKILL_WORKDIR
	mkdir -p "$work"
else
	work=$(mktemp -d)
	trap 'rm -rf "$work"' EXIT
fi

awk -v dir="$work" '
	/^```go$/ { block = ""; inside = 1; next }
	/^```$/ && inside {
		inside = 0
		if (block ~ /^package main/) {
			n++
			path = dir "/p" n
			system("mkdir -p " path)
			printf "%s", block > (path "/main.go")
			close(path "/main.go")
		}
		next
	}
	inside { block = block $0 "\n" }
' "$skill"

count=$(find "$work" -name main.go | wc -l)
if [ "$count" -eq 0 ]; then
	echo "no programs in $skill" >&2
	exit 1
fi
cat > "$work/go.mod" <<MOD
module skillcheck

go 1.21

require github.com/insei/tinyconf/v2 v2.0.0

replace github.com/insei/tinyconf/v2 => $root
MOD
cp "$root/go.sum" "$work/go.sum"
cd "$work"
go mod tidy >/dev/null
go vet ./...
go build ./...
echo "$count programs of $(basename "$(dirname "$skill")") skill are built"

#!/usr/bin/env bash
# Everything that has to hold before a change is finished. CI runs this same
# script, so what passes here passes there for the same reasons.
#
#   check.sh        report anything that does not hold
#   check.sh fix    apply what can be applied, then report the rest
#
# The tools are pinned. An unpinned linter turns somebody else's release into
# a failure on an unrelated change.

set -euo pipefail

readonly staticcheck=honnef.co/go/tools/cmd/staticcheck@v0.8.1
readonly modernize=golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@v0.23.0

# The command modules are built and vetted on the platforms that can build
# them, so only formatting is checked here, where every file is readable
# whatever it is written for.
readonly modules=(. ./cmd/preview ./cmd/shell-extension)

cd "$(dirname "$0")"

fix=false
case ${1:-} in
"") ;;
fix) fix=true ;;
*)
	echo "usage: check.sh [fix]" >&2
	exit 1
	;;
esac

failed=0

# report runs a step, letting it fail without stopping the ones after it, so
# one run shows everything rather than the first thing.
report() {
	local what=$1
	shift
	if ! "$@"; then
		echo "  ^ $what" >&2
		failed=1
	fi
}

formatting() {
	local unformatted
	unformatted="$(gofmt -l "${modules[@]}")"
	if [[ -n $unformatted ]]; then
		echo "$unformatted"
		return 1
	fi
}

# An analyser that reports through its output rather than its status needs
# its output turned into one.
analyse() {
	local output
	output="$(go run "$@" ./... 2>&1)" || true
	if [[ -n $output ]]; then
		echo "$output"
		return 1
	fi
}

if $fix; then
	gofmt -w "${modules[@]}" > /dev/null
	go run "$modernize" -fix ./... > /dev/null 2>&1 || true
fi

report "formatting: run check.sh fix" formatting
report "vet" go vet ./...
report "staticcheck: dead code and suspect constructs" analyse "$staticcheck"
report "modernize: run check.sh fix" analyse "$modernize"
report "tests" go test -count=1 ./...

if ((failed)); then
	echo >&2
	echo "check failed" >&2
	exit 1
fi
echo "check passed"

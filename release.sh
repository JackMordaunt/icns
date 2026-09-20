#!/usr/bin/env bash
# Publish a release with goreleaser.
#
#   release.sh [tag]      releases a tag that already exists, by default the
#                         one at HEAD
#   release.sh mint tag   tags HEAD and releases that
#
# The tag reaches origin before goreleaser runs, because GitHub creates a
# missing tag at the default branch tip rather than at the commit released.

set -euo pipefail

usage="usage: release.sh [tag] | release.sh mint tag"

die() {
	echo "$*" >&2
	exit 1
}

mint=false
case ${1:-} in
mint)
	mint=true
	tag=${2:-}
	[[ -n $tag && $# -eq 2 ]] || die "$usage"
	;;
*)
	tag=${1:-}
	[[ $# -le 1 ]] || die "$usage"
	;;
esac

mapfile -t at_head < <(git tag --points-at HEAD)

if $mint; then
	[[ ${#at_head[@]} -eq 0 ]] ||
		die "HEAD already carries ${at_head[*]}; release it with: release.sh"
	! git rev-parse -q --verify "refs/tags/$tag" >/dev/null ||
		die "$tag already exists; release it with: release.sh $tag"
else
	if [[ -z $tag ]]; then
		case ${#at_head[@]} in
		0) die "commit not tagged; mint one with: release.sh mint TAG" ;;
		1) tag=${at_head[0]} ;;
		*) die "HEAD carries ${at_head[*]}; name the one to release" ;;
		esac
	elif ! git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
		die "$tag is not a tag; mint it with: release.sh mint $tag"
	fi
	[[ $(git rev-parse "$tag^{}") == "$(git rev-parse HEAD)" ]] ||
		die "$tag is not at HEAD; check it out to release it"
fi

[[ -z $(git status --porcelain) ]] || die "working tree is not clean"

command -v goreleaser >/dev/null || die "goreleaser is not installed"

# Origin must already have the commit: tagging one it lacks would release
# something nobody can check out.
git fetch --quiet origin main
git merge-base --is-ancestor HEAD FETCH_HEAD ||
	die "HEAD is not on origin/main; push the branch first"

! $mint || git tag -a "$tag" -m "$tag"

remote=$(git ls-remote --tags origin "refs/tags/$tag^{}" | cut -f1)
[[ -n $remote ]] || remote=$(git ls-remote --tags origin "refs/tags/$tag" | cut -f1)
here=$(git rev-parse "$tag^{}")

if [[ -z $remote ]]; then
	git push origin "refs/tags/$tag"
elif [[ $remote != "$here" ]]; then
	die "origin has $tag at $remote, not $here"
fi

GITHUB_TOKEN=$(pass goreleaser/github-token) goreleaser release --clean

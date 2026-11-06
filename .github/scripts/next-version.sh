#!/usr/bin/env bash
# Usage: next-version.sh <latest-tag>
#
# Prints the tag the commits since <latest-tag> call for, or nothing when they
# call for no release. A release takes one bump, the largest any commit asks
# for:
#
#   major  a "!" after the type ("feat!: ...", "fix(api)!: ...") or a
#          "BREAKING CHANGE: " / "BREAKING-CHANGE: " footer line
#   minor  feat
#   patch  fix
#
# Scopes count ("fix(ci): ..." is a fix); other types and merge subjects do not.
# A major bump from v1 or later needs go.mod's module path to end in the new
# /vN, because Go cannot fetch v2.0.0 of a module whose path has no /v2.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 <latest-tag>" >&2
	exit 2
fi
latest=$1
IFS=. read -r major minor patch <<<"${latest#v}"

header_re='^([a-z]+)(\([^()]*\))?(!)?: '
breaking_footer_re='^BREAKING[ -]CHANGE: '
bump=0 # 0 none, 1 patch, 2 minor, 3 major
while IFS= read -r -d '' message || [[ -n $message ]]; do
	subject=${message%%$'\n'*}
	if [[ ! $subject =~ $header_re ]]; then
		continue
	fi
	type=${BASH_REMATCH[1]}
	level=0
	if [[ -n ${BASH_REMATCH[3]} ]] || grep -qE "$breaking_footer_re" <<<"$message"; then
		level=3
	elif [[ $type == feat ]]; then
		level=2
	elif [[ $type == fix ]]; then
		level=1
	fi
	if ((level > bump)); then
		bump=$level
	fi
done < <(git log -z --format=%B "${latest}..HEAD")

case $bump in
0)
	exit 0
	;;
1)
	patch=$((patch + 1))
	;;
2)
	minor=$((minor + 1))
	patch=0
	;;
3)
	major=$((major + 1))
	minor=0
	patch=0
	module=$(awk '$1 == "module" { print $2; exit }' go.mod)
	if ((major >= 2)) && [[ $module != */v$major ]]; then
		echo "a breaking change since $latest calls for v$major.0.0, but go.mod's module path $module has no /v$major, so Go could not fetch that tag" >&2
		exit 1
	fi
	;;
esac
printf 'v%d.%d.%d\n' "$major" "$minor" "$patch"

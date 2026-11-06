#!/usr/bin/env bash
# Tests next-version.sh against throwaway repositories, one per case, each
# tagged v1.6.10 before the commits under test.
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/next-version.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Keep the test independent of the caller's git configuration.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.invalid

v1=github.com/Infra-Forge/infra-apix
failures=0

# check <name> <module path> <want> <commit message>...
# <want> is the expected output, or "error: <text>" for a failure whose
# message contains <text>.
check() {
	local name=$1 module=$2 want=$3
	shift 3
	local dir="$tmp/$name" message got status=0
	git init -q -b main "$dir"
	printf 'module %s\n\ngo 1.27.3\n' "$module" >"$dir/go.mod"
	git -C "$dir" add go.mod
	git -C "$dir" commit -q -m "chore: start"
	git -C "$dir" tag v1.6.10
	for message in "$@"; do
		git -C "$dir" commit -q --allow-empty -m "$message"
	done
	got=$(cd "$dir" && "$script" v1.6.10 2>&1) || status=$?
	if [[ $want == "error: "* ]]; then
		if ((status != 0)) && [[ $got == *"${want#error: }"* ]]; then
			echo "ok   $name"
			return
		fi
	elif ((status == 0)) && [[ $got == "$want" ]]; then
		echo "ok   $name"
		return
	fi
	echo "FAIL $name: exit $status, output \"$got\", want \"$want\""
	failures=$((failures + 1))
}

check no-commits "$v1" ""
check no-release-types "$v1" "" "docs: a" "ci: b" "chore: c" "test: d" "refactor: e"
check merge-subjects "$v1" "" "Merge pull request #1 from org/fix/x" "Merge branch 'main' into feat/y"
check one-fix "$v1" v1.6.11 "fix: a"
check two-fixes-one-bump "$v1" v1.6.11 "fix: a" "fix: b"
check scoped-fix "$v1" v1.6.11 "fix(ci): a"
check feat-and-fixes "$v1" v1.7.0 "fix: a" "feat: b" "fix: c"
check two-feats-one-bump "$v1" v1.7.0 "feat: a" "feat(runtime): b"
check uppercase-type "$v1" "" "Fix: a"
check footer-text-mid-line "$v1" v1.6.11 $'fix: a\n\nExplains the BREAKING CHANGE: footer.'
check bang-on-v1 "$v1" "error: has no /v2" "feat!: a"
check scoped-bang-on-v1 "$v1" "error: has no /v2" "fix(api)!: a"
check footer-on-v1 "$v1" "error: has no /v2" $'fix: a\n\nBREAKING CHANGE: b'
check hyphen-footer-on-v1 "$v1" "error: has no /v2" $'feat: a\n\nBREAKING-CHANGE: b'
check breaking-on-v2-path "$v1/v2" v2.0.0 "fix: a" "feat!: b"

# reject <tag>: next-version.sh refuses <tag> before reading any commit.
reject() {
	local out
	if out=$("$script" "$1" 2>&1); then
		echo "FAIL tag $1: accepted, output \"$out\""
		failures=$((failures + 1))
	elif [[ $out != *"unsupported tag $1"* ]]; then
		echo "FAIL tag $1: output \"$out\""
		failures=$((failures + 1))
	else
		echo "ok   tag $1 refused"
	fi
}

reject v1.7.0-rc.1
reject v1.6.10+build.5
reject 1.6.10
reject v1.06.0

if out=$("$script" 2>&1); then
	echo "FAIL usage: exit 0 without a tag"
	failures=$((failures + 1))
elif [[ $out != usage:* ]]; then
	echo "FAIL usage: output \"$out\""
	failures=$((failures + 1))
else
	echo "ok   usage"
fi

if ((failures > 0)); then
	echo "$failures case(s) failed"
	exit 1
fi

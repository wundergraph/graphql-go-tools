#!/bin/sh

set -eu

GOLANGCI_LINT_VERSION=${GOLANGCI_LINT_VERSION:-v2.12.2}

validate_version() {
	if ! printf '%s\n' "$GOLANGCI_LINT_VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$'; then
		echo "invalid GOLANGCI_LINT_VERSION: $GOLANGCI_LINT_VERSION" >&2
		exit 1
	fi
}

validate_version

if [ "${1:-}" = "--print-version" ]; then
	printf '%s\n' "$GOLANGCI_LINT_VERSION"
	exit 0
fi

install_golangci_lint() {
	mkdir -p "$install_dir"
	installer="$install_dir/install.sh.$$"
	trap 'rm -f "$installer"' EXIT HUP INT TERM

	if command -v curl >/dev/null 2>&1; then
		curl -sSfL https://golangci-lint.run/install.sh -o "$installer"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$installer" https://golangci-lint.run/install.sh
	else
		echo "installing golangci-lint requires curl or wget" >&2
		exit 1
	fi

	sh "$installer" -b "$install_dir" "$GOLANGCI_LINT_VERSION"
	rm -f "$installer"
	trap - EXIT HUP INT TERM
}

install_dir="${TMPDIR:-/tmp}/golangci-lint/$GOLANGCI_LINT_VERSION"
binary="$install_dir/golangci-lint"

if [ ! -x "$binary" ]; then
	echo "installing golangci-lint $GOLANGCI_LINT_VERSION in $install_dir" >&2
	install_golangci_lint
fi

exec "$binary" "$@"

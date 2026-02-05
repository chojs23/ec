#!/bin/sh
set -eu

APP="ec"
ALIAS="easy-conflict"
OWNER="chojs23"
REPO="ec"
PREFIX="${PREFIX:-$HOME/.local}"
BIN_DIR="$PREFIX/bin"
VERSION="${VERSION:-latest}"

require_command() {
	if ! command -v "$1" >/dev/null 2>&1; then
		printf "%s is required\n" "$1" >&2
		exit 1
	fi
}

require_command curl

case "$(uname -s)" in
Darwin)
	os="darwin"
	;;
Linux)
	os="linux"
	;;
*)
	printf "Unsupported OS: %s\n" "$(uname -s)" >&2
	exit 1
	;;
esac

case "$(uname -m)" in
x86_64 | amd64)
	arch="amd64"
	;;
arm64 | aarch64)
	arch="arm64"
	;;
*)
	printf "Unsupported architecture: %s\n" "$(uname -m)" >&2
	exit 1
	;;
esac

asset="${APP}-${os}-${arch}"

if [ "$VERSION" = "latest" ]; then
	base_url="https://github.com/${OWNER}/${REPO}/releases/latest/download"
	tag_label="latest"
else
	case "$VERSION" in
	v*)
		tag="$VERSION"
		;;
	*)
		tag="v${VERSION}"
		;;
	esac
	base_url="https://github.com/${OWNER}/${REPO}/releases/download/${tag}"
	tag_label="$tag"
fi

tmp_dir="$(mktemp -d)"
cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT

asset_path="$tmp_dir/$asset"
checksums_path="$tmp_dir/checksums.txt"

curl -fsSL "$base_url/$asset" -o "$asset_path"
curl -fsSL "$base_url/checksums.txt" -o "$checksums_path"

expected_checksum="$(awk -v file="$asset" '$2 == file { print $1 }' "$checksums_path")"
if [ -z "$expected_checksum" ]; then
	printf "Checksum not found for %s in %s\n" "$asset" "$tag_label" >&2
	exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
	actual_checksum="$(sha256sum "$asset_path" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
	actual_checksum="$(shasum -a 256 "$asset_path" | awk '{ print $1 }')"
else
	printf "sha256sum or shasum is required\n" >&2
	exit 1
fi

if [ "$expected_checksum" != "$actual_checksum" ]; then
	printf "Checksum mismatch for %s\n" "$asset" >&2
	exit 1
fi

mkdir -p "$BIN_DIR"
mv "$asset_path" "$BIN_DIR/$APP"
chmod 0755 "$BIN_DIR/$APP"
ln -sf "$BIN_DIR/$APP" "$BIN_DIR/$ALIAS"

printf "Installed %s to %s\n" "$APP" "$BIN_DIR/$APP"
printf "Installed %s to %s\n" "$ALIAS" "$BIN_DIR/$ALIAS"

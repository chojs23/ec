#!/bin/sh
set -eu

APP="ec"
ALIAS="easy-conflict"
PREFIX="${PREFIX:-$HOME/.local}"
BIN_DIR="$PREFIX/bin"

mkdir -p "$BIN_DIR"
go build -o "$BIN_DIR/$APP" ./cmd/ec
ln -sf "$BIN_DIR/$APP" "$BIN_DIR/$ALIAS"

printf "Installed %s to %s\n" "$APP" "$BIN_DIR/$APP"
printf "Installed %s to %s\n" "$ALIAS" "$BIN_DIR/$ALIAS"

#!/bin/sh
set -eu

APP="easy-conflict"
PREFIX="${PREFIX:-$HOME/.local}"
BIN_DIR="$PREFIX/bin"

mkdir -p "$BIN_DIR"
go build -o "$BIN_DIR/$APP" ./cmd/easy-conflict

printf "Installed %s to %s\n" "$APP" "$BIN_DIR/$APP"

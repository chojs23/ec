# ec (easy-conflict)

Easy terminal-native 3-way git mergetool vim-like workflow

This package is a thin installer for the prebuilt `ec` Go binary. On
install it downloads the matching release binary from GitHub, verifies
the SHA-256 checksum, and places it under the package directory.

## Install

```bash
npm install -g @chojs23/ec
```

Or run without installing:

```bash
npx @chojs23/ec
```

## Supported platforms

- macOS (x64, arm64)
- Linux (x64, arm64)
- Windows (x64, arm64)

## Environment

- `EC_SKIP_DOWNLOAD=1` skips the postinstall download. Use this in
  environments where network access is restricted; you must then place
  an `ec` binary in the package's `vendor/` directory yourself.

## Links

- Source and docs: https://github.com/chojs23/ec
- Issues: https://github.com/chojs23/ec/issues

## License

MIT

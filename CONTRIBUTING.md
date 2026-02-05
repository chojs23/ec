# Contributing

New features and bug reports are welcome.
We appreciate issues and pull requests.

## Pull requests

1. Keep changes focused and small when possible.
2. Link the related issue if one exists.
3. Run build, tests, and formatting before opening the PR.

## Development setup

1. Install Go 1.24.5.
2. Clone the repo and change into the directory.
3. Ensure git is available on your PATH for tests.

## Build

Build the binary and create the symlink.

```
make build
```

Build output:

1. ./ec

## Test

Run all tests.

```
make test
```

## Format

Format all Go files in the repo.

```
make fmt
```

Format only the files you changed.

```
gofmt -w path/to/file.go
```

Check formatting without changing files.

```
make fmt-check
```

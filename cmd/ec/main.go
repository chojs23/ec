package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/run"
)

var version = "dev"

func main() {
	ctx := context.Background()
	opts, err := cli.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, cli.ErrHelp) {
			fmt.Fprintln(os.Stdout, cli.Usage())
			os.Exit(0)
		}
		if errors.Is(err, cli.ErrVersion) {
			fmt.Fprintf(os.Stdout, "ec %s\n", versionString())
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	exitCode := run.Run(ctx, opts)
	os.Exit(exitCode)
}

func versionString() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version
	}
	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		return version
	}
	return info.Main.Version
}

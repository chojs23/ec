package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/run"
)

func main() {
	ctx := context.Background()
	opts, err := cli.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, cli.ErrHelp) {
			fmt.Fprintln(os.Stdout, cli.Usage())
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	exitCode := run.Run(ctx, opts)
	os.Exit(exitCode)
}

package run

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/engine"
	"github.com/chojs23/ec/internal/tui"
)

func Run(ctx context.Context, opts cli.Options) int {
	if opts.Check {
		resolved, err := engine.CheckResolvedFile(opts.MergedPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if resolved {
			return 0
		}
		return 1
	}

	if opts.ApplyAll != "" {
		if err := engine.ApplyAllAndWrite(ctx, opts); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		return 0
	}

	// Interactive TUI
	if opts.BasePath == "" && opts.LocalPath == "" && opts.RemotePath == "" && opts.MergedPath == "" {
		baseOpts := opts
		for {
			opts = baseOpts
			cleanup, err := prepareInteractiveFromRepo(ctx, &opts)
			if err != nil {
				if errors.Is(err, errNoConflicts) {
					fmt.Fprintln(os.Stdout, "No conflicted files found in the current directory.")
					return 0
				}
				if errors.Is(err, tui.ErrSelectorQuit) {
					return 0
				}
				fmt.Fprintln(os.Stderr, err)
				return 2
			}

			err = tui.Run(ctx, opts)
			cleanup()
			if err != nil {
				if errors.Is(err, tui.ErrBackToSelector) {
					continue
				}
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
			return 0
		}
	}

	if err := tui.Run(ctx, opts); err != nil {
		if errors.Is(err, tui.ErrBackToSelector) {
			return 0
		}
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func exitNotImplemented(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return 2
}

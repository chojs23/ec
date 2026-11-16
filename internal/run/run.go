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

var runResolver = tui.Run

func Run(ctx context.Context, opts cli.Options) int {
	if opts.Check {
		return resolvedExitCode(opts.MergedPath)
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
		if isInteractiveTTY() {
			err := tui.RunWorkspace(ctx, func(ctx context.Context) (tui.WorkspaceData, error) {
				return loadWorkspaceFromRepo(ctx, opts)
			})
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
			return 0
		}

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

			err = runResolver(ctx, opts)
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

	opts.ConfirmIncompleteQuit = true
	if err := runResolver(ctx, opts); err != nil && !errors.Is(err, tui.ErrBackToSelector) {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return resolvedExitCode(opts.MergedPath)
}

func resolvedExitCode(mergedPath string) int {
	resolved, err := engine.CheckResolvedFile(mergedPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if resolved {
		return 0
	}
	return 1
}

package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

var ErrHelp = errors.New("help requested")
var ErrVersion = errors.New("version requested")

func Parse(args []string) (Options, error) {
	var opts Options
	var help bool
	var backup bool
	var showVersion bool

	opts.Backup = false

	fs := flag.NewFlagSet("ec", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&opts.BasePath, "base", "", "Path to BASE (ancestor) file")
	fs.StringVar(&opts.LocalPath, "local", "", "Path to LOCAL (ours) file")
	fs.StringVar(&opts.RemotePath, "remote", "", "Path to REMOTE (theirs) file")
	fs.StringVar(&opts.MergedPath, "merged", "", "Path to MERGED file (output target)")
	fs.StringVar(&opts.ApplyAll, "apply-all", "", "Non-interactive resolution: ours|theirs|both")
	fs.BoolVar(&opts.Check, "check", false, "Exit 0 if resolved (no conflict markers), else 1")
	fs.BoolVar(&backup, "backup", false, "Create $MERGED.ec.bak on write")
	fs.BoolVar(&opts.Verbose, "v", false, "Verbose logging to stderr")
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.BoolVar(&showVersion, "version", false, "Show version")

	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		return Options{}, fmt.Errorf("%w\n\n%s", err, Usage())
	}
	if help {
		return Options{}, ErrHelp
	}
	if showVersion {
		return Options{}, ErrVersion
	}

	if backup {
		opts.Backup = true
	}

	// Positional mergetool form: <BASE> <LOCAL> <REMOTE> <MERGED>
	if opts.BasePath == "" && opts.LocalPath == "" && opts.RemotePath == "" && opts.MergedPath == "" {
		if fs.NArg() == 4 {
			opts.BasePath = fs.Arg(0)
			opts.LocalPath = fs.Arg(1)
			opts.RemotePath = fs.Arg(2)
			opts.MergedPath = fs.Arg(3)
		}
	}

	opts.ApplyAll = strings.ToLower(strings.TrimSpace(opts.ApplyAll))
	if opts.ApplyAll != "" && opts.ApplyAll != "ours" && opts.ApplyAll != "theirs" && opts.ApplyAll != "both" && opts.ApplyAll != "none" {
		return Options{}, fmt.Errorf("invalid --apply-all: %q (expected ours|theirs|both|none)", opts.ApplyAll)
	}

	if opts.Check {
		// Only needs merged.
		if opts.MergedPath == "" {
			return Options{}, fmt.Errorf("--check requires --merged (or positional args)\n\n%s", Usage())
		}
		return opts, nil
	}

	if opts.ApplyAll != "" {
		if opts.BasePath == "" || opts.LocalPath == "" || opts.RemotePath == "" || opts.MergedPath == "" {
			return Options{}, fmt.Errorf("--apply-all requires base/local/remote/merged\n\n%s", Usage())
		}
		return opts, nil
	}

	// No-arg mode: detect conflicts in current repo and select a file.
	if opts.BasePath == "" && opts.LocalPath == "" && opts.RemotePath == "" && opts.MergedPath == "" {
		return opts, nil
	}

	// Interactive mode needs full paths.
	if opts.BasePath == "" || opts.LocalPath == "" || opts.RemotePath == "" || opts.MergedPath == "" {
		return Options{}, fmt.Errorf("missing required paths\n\n%s", Usage())
	}

	return opts, nil
}

func Usage() string {
	return strings.TrimSpace(`Usage:
	  ec
	  ec <BASE> <LOCAL> <REMOTE> <MERGED>
	  ec --base <path> --local <path> --remote <path> --merged <path>

Modes:
	  --check                     Exit 0 if $MERGED has no valid conflict blocks, else 1
	  --apply-all ours|theirs|both|none Resolve all conflicts non-interactively and write $MERGED

No-args mode:
	  If invoked with no paths and no mode flags, ec lists
	  conflicted files under the current directory and prompts to select one.

Options:
	  --backup                    Create $MERGED.ec.bak
	  --version                   Show version
	  -v                          Verbose logging
`)
}

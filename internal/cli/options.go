package cli

// Options is the fully-parsed configuration for a single invocation.
//
// It supports both:
// - mergetool-style positional args: <BASE> <LOCAL> <REMOTE> <MERGED>
// - standalone flags: --base/--local/--remote/--merged
type Options struct {
	BasePath   string
	LocalPath  string
	RemotePath string
	MergedPath string

	ApplyAll string // ours|theirs|both
	Check    bool

	NoBackup bool
	Verbose  bool

	AllowMissingBase bool
}

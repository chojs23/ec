package run

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/engine"
	"github.com/chojs23/ec/internal/gitutil"
	"github.com/chojs23/ec/internal/tui"
)

var errNoConflicts = errors.New("no conflicted files found")

const maxRecentDiffCommits = 100

type skippedConflict struct {
	path   string
	reason string
}

type repoWorkspace struct {
	repoRoot     string
	scope        string
	conflictPath []string
	stagesByPath map[string]map[int]gitutil.StageInfo
	skipped      []skippedConflict
}

func prepareInteractiveFromRepo(ctx context.Context, opts *cli.Options) (func(), error) {
	workspace, err := discoverRepoWorkspace(ctx)
	if err != nil {
		return nil, err
	}
	warnSkippedConflicts(workspace.skipped)

	if len(workspace.conflictPath) == 0 {
		if len(workspace.skipped) > 0 {
			return nil, fmt.Errorf("no supported conflicted files found in the current directory; skipped %s", formatSkippedConflicts(workspace.skipped))
		}
		return nil, errNoConflicts
	}

	selected, err := selectPathInteractive(ctx, workspace.repoRoot, workspace.conflictPath)
	if err != nil {
		return nil, err
	}
	return prepareConflictFromRepo(ctx, opts, workspace, selected)
}

func discoverRepoWorkspace(ctx context.Context) (repoWorkspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return repoWorkspace{}, fmt.Errorf("get working directory: %w", err)
	}

	repoRoot, err := gitutil.RepoRoot(ctx, cwd)
	if err != nil {
		return repoWorkspace{}, err
	}

	scope, err := filepath.Rel(repoRoot, cwd)
	if err != nil {
		scope = "."
	}
	scope = filepath.ToSlash(scope)

	paths, err := gitutil.ListUnmergedFiles(ctx, repoRoot, scope)
	if err != nil {
		return repoWorkspace{}, err
	}
	paths, stagesByPath, skipped, err := supportedConflictPaths(ctx, repoRoot, paths)
	if err != nil {
		return repoWorkspace{}, err
	}
	return repoWorkspace{
		repoRoot:     repoRoot,
		scope:        scope,
		conflictPath: paths,
		stagesByPath: stagesByPath,
		skipped:      skipped,
	}, nil
}

func loadWorkspaceFromRepo(ctx context.Context, baseOpts cli.Options) (tui.WorkspaceData, error) {
	workspace, err := discoverRepoWorkspace(ctx)
	if err != nil {
		return tui.WorkspaceData{}, err
	}
	var warnings bytes.Buffer
	writeSkippedConflicts(&warnings, workspace.skipped)
	conflicts, err := buildFileCandidates(workspace.repoRoot, workspace.conflictPath)
	if err != nil {
		return tui.WorkspaceData{}, err
	}
	diffSources, err := gitutil.RecentCommitSources(ctx, workspace.repoRoot, workspace.scope, maxRecentDiffCommits)
	if err != nil {
		if ctx.Err() != nil {
			return tui.WorkspaceData{}, ctx.Err()
		}
		if len(conflicts) == 0 {
			return tui.WorkspaceData{}, err
		}
		// Resolving index stages must not depend on optional commit history or statistics.
		fmt.Fprintf(&warnings, "Warning: recent commits are unavailable; continuing with conflicts and working tree: %v\n", err)
		diffSources = nil
	}
	diffSources = append([]gitutil.DiffSource{gitutil.WorkingTreeSource()}, diffSources...)
	data := tui.WorkspaceData{
		RepoRoot: workspace.repoRoot, Scope: workspace.scope,
		Conflicts: conflicts, DiffSources: diffSources,
		Warning: strings.TrimSpace(warnings.String()),
		PrepareConflict: func(ctx context.Context, selected string) (tui.PreparedConflict, error) {
			opts := baseOpts
			var warnings bytes.Buffer
			cleanup, err := prepareConflictWithWarnings(ctx, &opts, workspace, selected, &warnings)
			return tui.PreparedConflict{Options: opts, Cleanup: cleanup, Warning: strings.TrimSpace(warnings.String())}, err
		},
	}
	return data, nil
}

func prepareConflictFromRepo(ctx context.Context, opts *cli.Options, workspace repoWorkspace, selected string) (func(), error) {
	return prepareConflictWithWarnings(ctx, opts, workspace, selected, os.Stderr)
}

func prepareConflictWithWarnings(ctx context.Context, opts *cli.Options, workspace repoWorkspace, selected string, warnings io.Writer) (func(), error) {
	repoRoot := workspace.repoRoot
	stages, ok := workspace.stagesByPath[selected]
	if !ok {
		return nil, fmt.Errorf("selected conflicted file %q is no longer available", selected)
	}

	mergedPath := selected
	if !filepath.IsAbs(mergedPath) {
		mergedPath = filepath.Join(repoRoot, selected)
	}
	if _, err := os.Stat(mergedPath); err != nil {
		return nil, fmt.Errorf("cannot access merged file %s: %w", selected, err)
	}

	localBytes, err := gitutil.ShowStage(ctx, repoRoot, 2, selected)
	if err != nil {
		return nil, fmt.Errorf("missing ours stage for %s: %w", selected, err)
	}
	remoteBytes, err := gitutil.ShowStage(ctx, repoRoot, 3, selected)
	if err != nil {
		return nil, fmt.Errorf("missing theirs stage for %s: %w", selected, err)
	}

	allowMissingBase := false
	var baseBytes []byte
	if _, ok := stages[1]; !ok {
		allowMissingBase = true
		fmt.Fprintf(warnings, "Warning: base stage missing for %s; continuing without base view.\n", selected)
	} else {
		baseBytes, err = gitutil.ShowStage(ctx, repoRoot, 1, selected)
		if err != nil {
			return nil, fmt.Errorf("read base stage for %s: %w", selected, err)
		}
	}

	basePath, localPath, remotePath, cleanup, err := writeTempStages(baseBytes, localBytes, remoteBytes)
	if err != nil {
		return nil, err
	}

	opts.BasePath = basePath
	opts.LocalPath = localPath
	opts.RemotePath = remotePath
	opts.MergedPath = mergedPath
	opts.AllowMissingBase = allowMissingBase

	return cleanup, nil
}

func warnSkippedConflicts(skipped []skippedConflict) {
	writeSkippedConflicts(os.Stderr, skipped)
}

func writeSkippedConflicts(w io.Writer, skipped []skippedConflict) {
	for _, conflict := range skipped {
		fmt.Fprintf(w, "Warning: skipping unsupported conflict %q: %s.\n", conflict.path, conflict.reason)
	}
}

func supportedConflictPaths(ctx context.Context, repoRoot string, paths []string) ([]string, map[string]map[int]gitutil.StageInfo, []skippedConflict, error) {
	supported := make([]string, 0, len(paths))
	skipped := []skippedConflict{}
	stagesByPath := map[string]map[int]gitutil.StageInfo{}

	for _, path := range paths {
		stages, err := gitutil.UnmergedStages(ctx, repoRoot, path)
		if err != nil {
			return nil, nil, nil, err
		}
		if reason := unsupportedConflictReason(stages); reason != "" {
			skipped = append(skipped, skippedConflict{path: path, reason: reason})
			continue
		}

		supported = append(supported, path)
		stagesByPath[path] = stages
	}

	return supported, stagesByPath, skipped, nil
}

func unsupportedConflictReason(stages map[int]gitutil.StageInfo) string {
	if len(stages) == 0 {
		return "no unmerged index stages were found"
	}
	for _, stage := range stages {
		if stage.Mode == "160000" {
			return "submodule conflicts are not supported"
		}
	}
	if _, ok := stages[2]; !ok {
		return "delete/modify conflicts are not supported because the ours stage is missing"
	}
	if _, ok := stages[3]; !ok {
		return "delete/modify conflicts are not supported because the theirs stage is missing"
	}
	return ""
}

func formatSkippedConflicts(skipped []skippedConflict) string {
	parts := make([]string, 0, len(skipped))
	for _, conflict := range skipped {
		parts = append(parts, fmt.Sprintf("%q (%s)", conflict.path, conflict.reason))
	}
	return strings.Join(parts, ", ")
}

func selectPath(paths []string) (string, error) {
	if len(paths) == 1 {
		return paths[0], nil
	}

	fmt.Fprintln(os.Stdout, "Conflicted files:")
	for i, p := range paths {
		fmt.Fprintf(os.Stdout, "  %d) %s\n", i+1, p)
	}

	reader := bufio.NewReader(os.Stdin)
	for attempt := 0; attempt < 3; attempt++ {
		fmt.Fprintf(os.Stdout, "Select a file to resolve [1-%d]: ", len(paths))
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read selection: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx, err := strconv.Atoi(line)
		if err != nil || idx < 1 || idx > len(paths) {
			fmt.Fprintln(os.Stdout, "Invalid selection.")
			continue
		}
		return paths[idx-1], nil
	}

	return "", fmt.Errorf("invalid selection")
}

func selectPathInteractive(ctx context.Context, repoRoot string, paths []string) (string, error) {
	if isInteractiveTTY() {
		candidates, err := buildFileCandidates(repoRoot, paths)
		if err != nil {
			return "", err
		}
		return tui.SelectFile(ctx, candidates)
	}
	return selectPath(paths)
}

func isInteractiveTTY() bool {
	return isTTY(os.Stdin) && isTTY(os.Stdout)
}

func isTTY(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func buildFileCandidates(repoRoot string, paths []string) ([]tui.FileCandidate, error) {
	candidates := make([]tui.FileCandidate, 0, len(paths))
	for _, path := range paths {
		mergedPath := path
		if !filepath.IsAbs(mergedPath) {
			mergedPath = filepath.Join(repoRoot, path)
		}

		resolved, err := engine.CheckResolvedFile(mergedPath)
		if err != nil {
			resolved = false
		}
		candidates = append(candidates, tui.FileCandidate{Path: path, Resolved: resolved})
	}
	return candidates, nil
}

func writeTempStages(base, local, remote []byte) (string, string, string, func(), error) {
	baseFile, err := os.CreateTemp("", "ec-base-*")
	if err != nil {
		return "", "", "", nil, fmt.Errorf("create base temp file: %w", err)
	}
	basePath := baseFile.Name()
	if _, err := baseFile.Write(base); err != nil {
		baseFile.Close()
		os.Remove(basePath)
		return "", "", "", nil, fmt.Errorf("write base temp file: %w", err)
	}
	if err := baseFile.Close(); err != nil {
		os.Remove(basePath)
		return "", "", "", nil, fmt.Errorf("close base temp file: %w", err)
	}

	localFile, err := os.CreateTemp("", "ec-local-*")
	if err != nil {
		os.Remove(basePath)
		return "", "", "", nil, fmt.Errorf("create local temp file: %w", err)
	}
	localPath := localFile.Name()
	if _, err := localFile.Write(local); err != nil {
		localFile.Close()
		os.Remove(basePath)
		os.Remove(localPath)
		return "", "", "", nil, fmt.Errorf("write local temp file: %w", err)
	}
	if err := localFile.Close(); err != nil {
		os.Remove(basePath)
		os.Remove(localPath)
		return "", "", "", nil, fmt.Errorf("close local temp file: %w", err)
	}

	remoteFile, err := os.CreateTemp("", "ec-remote-*")
	if err != nil {
		os.Remove(basePath)
		os.Remove(localPath)
		return "", "", "", nil, fmt.Errorf("create remote temp file: %w", err)
	}
	remotePath := remoteFile.Name()
	if _, err := remoteFile.Write(remote); err != nil {
		remoteFile.Close()
		os.Remove(basePath)
		os.Remove(localPath)
		os.Remove(remotePath)
		return "", "", "", nil, fmt.Errorf("write remote temp file: %w", err)
	}
	if err := remoteFile.Close(); err != nil {
		os.Remove(basePath)
		os.Remove(localPath)
		os.Remove(remotePath)
		return "", "", "", nil, fmt.Errorf("close remote temp file: %w", err)
	}

	cleanup := func() {
		os.Remove(basePath)
		os.Remove(localPath)
		os.Remove(remotePath)
	}

	return basePath, localPath, remotePath, cleanup, nil
}

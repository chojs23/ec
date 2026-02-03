package run

import (
	"bufio"
	"context"
	"errors"
	"fmt"
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

func prepareInteractiveFromRepo(ctx context.Context, opts *cli.Options) (func(), error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	repoRoot, err := gitutil.RepoRoot(ctx, cwd)
	if err != nil {
		return nil, err
	}

	scope, err := filepath.Rel(repoRoot, cwd)
	if err != nil {
		scope = "."
	}
	scope = filepath.ToSlash(scope)

	paths, err := gitutil.ListUnmergedFiles(ctx, repoRoot, scope)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errNoConflicts
	}

	selected, err := selectPathInteractive(ctx, repoRoot, paths)
	if err != nil {
		return nil, err
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

	baseBytes, err := gitutil.ShowStage(ctx, repoRoot, 1, selected)
	allowMissingBase := false
	if err != nil {
		allowMissingBase = true
		baseBytes = nil
		fmt.Fprintf(os.Stderr, "Warning: base stage missing for %s; continuing without base view.\n", selected)
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
			return nil, fmt.Errorf("check resolved %s: %w", path, err)
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

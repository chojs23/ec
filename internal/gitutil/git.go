package gitutil

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RepoRoot returns the repository root directory for the given working directory.
func RepoRoot(ctx context.Context, cwd string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel failed: %w", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", fmt.Errorf("git rev-parse returned empty repo root")
	}
	return root, nil
}

// ListUnmergedFiles returns repo-relative paths of conflicted files under scopePathspec.
func ListUnmergedFiles(ctx context.Context, repoRoot string, scopePathspec string) ([]string, error) {
	pathspec := scopePathspec
	if pathspec == "" {
		pathspec = "."
	}

	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=U", "--", pathspec)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only --diff-filter=U failed: %w", err)
	}

	lines := bytes.Split(bytes.TrimSpace(output), []byte{'\n'})
	if len(lines) == 1 && len(lines[0]) == 0 {
		return nil, nil
	}

	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		p := strings.TrimSpace(string(line))
		if p == "" {
			continue
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// ShowStage reads a conflicted file content from the git index stage (1=base, 2=ours, 3=theirs).
func ShowStage(ctx context.Context, repoRoot string, stage int, path string) ([]byte, error) {
	ref := fmt.Sprintf(":%d:%s", stage, path)
	cmd := exec.CommandContext(ctx, "git", "show", ref)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s failed: %w", ref, err)
	}
	return output, nil
}

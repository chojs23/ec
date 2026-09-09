package gitutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type DiffSourceKind int

const (
	DiffSourceWorkingTree DiffSourceKind = iota + 1
	DiffSourceCommit
)

// DiffSource identifies the Git state shown by the read-only diff viewer.
type DiffSource struct {
	Kind       DiffSourceKind
	Commit     string
	BaseCommit string
	ShortHash  string
	Subject    string
	Deletions  int
	Additions  int
	StatsKnown bool
}

// DiffFile identifies one changed path and preserves both sides of a rename.
type DiffFile struct {
	Path      string
	OldPath   string
	Status    string
	Untracked bool
}

// StageInfo describes one unmerged index stage for a path.
type StageInfo struct {
	Mode  string
	Stage int
	Path  string
}

// RepoRoot returns the repository root directory for the given working directory.
func RepoRoot(ctx context.Context, cwd string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return "", gitCommandError("git rev-parse --show-toplevel", err, stderr.String())
	}
	root := trimCommandLineEnding(string(output))
	if root == "" {
		return "", fmt.Errorf("git rev-parse returned empty repo root")
	}
	return root, nil
}

// ListUnmergedFiles returns repo-relative paths of conflicted files under scopePathspec.
func ListUnmergedFiles(ctx context.Context, repoRoot string, scopePathspec string) ([]string, error) {
	pathspec := literalPathspec(scopePathspec)

	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "-z", "--diff-filter=U", "--", pathspec)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, gitCommandError("git diff --name-only -z --diff-filter=U", err, stderr.String())
	}

	return splitNULPaths(output), nil
}

// UnmergedStages returns all unmerged index stages for a single repo-relative path.
func UnmergedStages(ctx context.Context, repoRoot string, path string) (map[int]StageInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-u", "-z", "--", literalPathspec(path))
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, gitCommandError("git ls-files -u -z", err, stderr.String())
	}

	stages := map[int]StageInfo{}
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}

		meta, rawPath, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("git ls-files -u returned malformed record %q", string(record))
		}

		fields := strings.Fields(string(meta))
		if len(fields) != 3 {
			return nil, fmt.Errorf("git ls-files -u returned malformed metadata %q", string(meta))
		}

		stage, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("git ls-files -u returned invalid stage %q: %w", fields[2], err)
		}

		stages[stage] = StageInfo{Mode: fields[0], Stage: stage, Path: string(rawPath)}
	}
	return stages, nil
}

// ShowStage reads a conflicted file content from the git index stage (1=base, 2=ours, 3=theirs).
func ShowStage(ctx context.Context, repoRoot string, stage int, path string) ([]byte, error) {
	ref := fmt.Sprintf(":%d:%s", stage, path)
	cmd := exec.CommandContext(ctx, "git", "show", ref)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, gitCommandError(fmt.Sprintf("git show %s", ref), err, stderr.String())
	}
	return output, nil
}

// WorkingTreeSource returns the source used for current staged, unstaged, and
// untracked changes.
func WorkingTreeSource() DiffSource {
	return DiffSource{Kind: DiffSourceWorkingTree}
}

// RecentCommitSources returns recent commits that touched the requested scope.
// Merge commits use their first parent because that matches the usual log view.
func RecentCommitSources(ctx context.Context, repoRoot string, scopePathspec string, limit int) ([]DiffSource, error) {
	if limit <= 0 {
		return nil, nil
	}

	_, hasHead, err := headCommit(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	if !hasHead {
		return nil, nil
	}

	format := "%H%x00%h%x00%P%x00%s"
	args := []string{"log", "-z", fmt.Sprintf("-n%d", limit), "--format=" + format, "--", literalPathspec(scopePathspec)}
	output, err := gitOutput(ctx, repoRoot, "git log", args...)
	if err != nil {
		return nil, err
	}

	fields := splitNULFields(output)
	if len(fields)%4 != 0 {
		return nil, fmt.Errorf("git log returned malformed commit records")
	}

	sources := make([]DiffSource, 0, len(fields)/4)
	emptyTree := ""
	for index := 0; index < len(fields); index += 4 {
		parent := ""
		if parents := strings.Fields(fields[index+2]); len(parents) > 0 {
			parent = parents[0]
		} else {
			if emptyTree == "" {
				emptyTree, err = gitEmptyTreeHash(ctx, repoRoot)
				if err != nil {
					return nil, err
				}
			}
			parent = emptyTree
		}
		sources = append(sources, DiffSource{
			Kind:       DiffSourceCommit,
			Commit:     fields[index],
			ShortHash:  fields[index+1],
			BaseCommit: parent,
			Subject:    fields[index+3],
		})
	}
	if err := loadCommitDiffTotals(ctx, repoRoot, scopePathspec, sources); err != nil {
		return nil, err
	}
	return sources, nil
}

func loadCommitDiffTotals(ctx context.Context, repoRoot string, scopePathspec string, sources []DiffSource) error {
	if len(sources) == 0 {
		return nil
	}

	commits := make([]string, 0, len(sources))
	for _, source := range sources {
		commits = append(commits, source.Commit)
	}
	cmd := exec.CommandContext(
		ctx,
		"git",
		"diff-tree",
		"--stdin",
		"--root",
		"--diff-merges=first-parent",
		"--find-renames",
		"--numstat",
		"-z",
		"--",
		literalPathspec(scopePathspec),
	)
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(strings.Join(commits, "\n") + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return gitCommandError("git diff-tree --stdin --numstat", err, stderr.String())
	}
	return applyCommitDiffTotals(output, sources)
}

func applyCommitDiffTotals(output []byte, sources []DiffSource) error {
	fields := splitNULFields(output)
	fieldIndex := 0
	for sourceIndex := range sources {
		if fieldIndex >= len(fields) || fields[fieldIndex] != sources[sourceIndex].Commit {
			return fmt.Errorf("git diff-tree returned malformed commit statistics")
		}
		fieldIndex++
		sources[sourceIndex].StatsKnown = true

		nextCommit := ""
		if sourceIndex+1 < len(sources) {
			nextCommit = sources[sourceIndex+1].Commit
		}
		for fieldIndex < len(fields) && (nextCommit == "" || fields[fieldIndex] != nextCommit) {
			parts := strings.SplitN(strings.TrimPrefix(fields[fieldIndex], "\n"), "\t", 3)
			if len(parts) != 3 {
				return fmt.Errorf("git diff-tree returned malformed numstat record")
			}

			additions, err := parseNumstatCount(parts[0])
			if err != nil {
				return err
			}
			deletions, err := parseNumstatCount(parts[1])
			if err != nil {
				return err
			}
			sources[sourceIndex].Additions += additions
			sources[sourceIndex].Deletions += deletions
			fieldIndex++

			if parts[2] == "" {
				if fieldIndex+1 >= len(fields) {
					return fmt.Errorf("git diff-tree returned malformed rename statistics")
				}
				fieldIndex += 2
			}
		}
	}
	if fieldIndex != len(fields) {
		return fmt.Errorf("git diff-tree returned unexpected statistics")
	}
	return nil
}

func parseNumstatCount(value string) (int, error) {
	if value == "-" {
		return 0, nil
	}
	count, err := strconv.Atoi(value)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("git diff-tree returned invalid line count %q", value)
	}
	return count, nil
}

// ListDiffFiles returns the paths changed by a working tree or commit source.
func ListDiffFiles(ctx context.Context, repoRoot string, source DiffSource, scopePathspec string) ([]DiffFile, error) {
	switch source.Kind {
	case DiffSourceWorkingTree:
		return listWorkingTreeDiffFiles(ctx, repoRoot, scopePathspec)
	case DiffSourceCommit:
		if source.Commit == "" || source.BaseCommit == "" {
			return nil, fmt.Errorf("commit diff source is incomplete")
		}
		args := []string{
			"diff", "--name-status", "-z", "--find-renames",
			source.BaseCommit, source.Commit, "--", literalPathspec(scopePathspec),
		}
		output, err := gitOutput(ctx, repoRoot, "git diff --name-status", args...)
		if err != nil {
			return nil, err
		}
		return parseNameStatus(output)
	default:
		return nil, fmt.Errorf("unknown diff source kind %d", source.Kind)
	}
}

// DiffFilePatch returns a no-color patch for one selected file.
func DiffFilePatch(ctx context.Context, repoRoot string, source DiffSource, file DiffFile) ([]byte, error) {
	if file.Path == "" {
		return nil, fmt.Errorf("diff file path is empty")
	}

	switch source.Kind {
	case DiffSourceWorkingTree:
		if file.Untracked {
			return untrackedFilePatch(ctx, repoRoot, file.Path)
		}

		_, hasHead, err := headCommit(ctx, repoRoot)
		if err != nil {
			return nil, err
		}
		baseCommit := "HEAD"
		if !hasHead {
			baseCommit, err = gitEmptyTreeHash(ctx, repoRoot)
			if err != nil {
				return nil, err
			}
		}
		args := []string{"diff", "--no-color", "--no-ext-diff", "--find-renames", baseCommit, "--"}
		args = append(args, diffFilePathspecs(file)...)
		return gitOutput(ctx, repoRoot, "git diff", args...)
	case DiffSourceCommit:
		if source.Commit == "" || source.BaseCommit == "" {
			return nil, fmt.Errorf("commit diff source is incomplete")
		}
		args := []string{
			"diff", "--no-color", "--no-ext-diff", "--find-renames",
			source.BaseCommit, source.Commit, "--",
		}
		args = append(args, diffFilePathspecs(file)...)
		return gitOutput(ctx, repoRoot, "git diff", args...)
	default:
		return nil, fmt.Errorf("unknown diff source kind %d", source.Kind)
	}
}

func listWorkingTreeDiffFiles(ctx context.Context, repoRoot string, scopePathspec string) ([]DiffFile, error) {
	_, hasHead, err := headCommit(ctx, repoRoot)
	if err != nil {
		return nil, err
	}

	baseCommit := "HEAD"
	if !hasHead {
		baseCommit, err = gitEmptyTreeHash(ctx, repoRoot)
		if err != nil {
			return nil, err
		}
	}
	args := []string{"diff", "--name-status", "-z", "--find-renames", baseCommit}
	args = append(args, "--", literalPathspec(scopePathspec))

	output, err := gitOutput(ctx, repoRoot, "git diff --name-status", args...)
	if err != nil {
		return nil, err
	}
	files, err := parseNameStatus(output)
	if err != nil {
		return nil, err
	}

	untrackedOutput, err := gitOutput(
		ctx,
		repoRoot,
		"git ls-files --others",
		"ls-files", "--others", "--exclude-standard", "-z", "--", literalPathspec(scopePathspec),
	)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		seen[file.Path] = struct{}{}
	}
	for _, path := range splitNULPaths(untrackedOutput) {
		if _, ok := seen[path]; ok {
			continue
		}
		files = append(files, DiffFile{Path: path, Status: "?", Untracked: true})
	}
	return files, nil
}

func parseNameStatus(output []byte) ([]DiffFile, error) {
	fields := splitNULFields(output)
	files := make([]DiffFile, 0, len(fields)/2)
	for index := 0; index < len(fields); {
		status := fields[index]
		index++
		if status == "" {
			return nil, fmt.Errorf("git diff returned an empty file status")
		}

		if status[0] == 'R' || status[0] == 'C' {
			if index+1 >= len(fields) {
				return nil, fmt.Errorf("git diff returned malformed rename record")
			}
			files = append(files, DiffFile{
				Status:  status,
				OldPath: fields[index],
				Path:    fields[index+1],
			})
			index += 2
			continue
		}

		if index >= len(fields) {
			return nil, fmt.Errorf("git diff returned malformed file record")
		}
		files = append(files, DiffFile{Status: status, Path: fields[index]})
		index++
	}
	return files, nil
}

func diffFilePathspecs(file DiffFile) []string {
	paths := make([]string, 0, 2)
	if file.OldPath != "" && file.OldPath != file.Path {
		paths = append(paths, literalPathspec(file.OldPath))
	}
	return append(paths, literalPathspec(file.Path))
}

func untrackedFilePatch(ctx context.Context, repoRoot string, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--no-color", "--", os.DevNull, path)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err == nil {
		return output, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return output, nil
	}
	return nil, gitCommandError("git diff --no-index", err, stderr.String())
}

func headCommit(ctx context.Context, repoRoot string) (string, bool, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", "HEAD")
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err == nil {
		return trimCommandLineEnding(string(output)), true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", false, nil
	}
	return "", false, gitCommandError("git rev-parse --verify --quiet HEAD", err, stderr.String())
}

func gitEmptyTreeHash(ctx context.Context, repoRoot string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "hash-object", "-t", "tree", "--stdin")
	cmd.Dir = repoRoot
	cmd.Stdin = bytes.NewReader(nil)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return "", gitCommandError("git hash-object -t tree --stdin", err, stderr.String())
	}
	hash := trimCommandLineEnding(string(output))
	if hash == "" {
		return "", fmt.Errorf("git hash-object returned an empty tree hash")
	}
	return hash, nil
}

func gitOutput(ctx context.Context, repoRoot string, command string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, gitCommandError(command, err, stderr.String())
	}
	return output, nil
}

func splitNULPaths(output []byte) []string {
	if len(output) == 0 {
		return nil
	}

	parts := bytes.Split(output, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		paths = append(paths, string(part))
	}
	return paths
}

func splitNULFields(output []byte) []string {
	parts := bytes.Split(output, []byte{0})
	if len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}

	fields := make([]string, 0, len(parts))
	for _, part := range parts {
		fields = append(fields, string(part))
	}
	return fields
}

func literalPathspec(pathspec string) string {
	if pathspec == "" || pathspec == "." {
		return "."
	}
	return ":(literal)" + pathspec
}

func trimCommandLineEnding(output string) string {
	output = strings.TrimSuffix(output, "\n")
	return strings.TrimSuffix(output, "\r")
}

func gitCommandError(command string, err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("%s failed: %w", command, err)
	}
	return fmt.Errorf("%s failed: %s: %w", command, stderr, err)
}

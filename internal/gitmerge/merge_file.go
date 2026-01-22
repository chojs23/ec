package gitmerge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// MergeFileDiff3 runs git's canonical three-way merge and returns a diff3-style
// merge view (with base sections in conflict blocks).
//
// Exit code 0 means clean merge. Any positive exit code indicates the number of
// conflicts found (truncated to 127 if >127). Negative exit codes indicate errors.
func MergeFileDiff3(ctx context.Context, localPath, basePath, remotePath string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "merge-file", "--diff3", "-p", localPath, basePath, remotePath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}

	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code := ee.ExitCode()
		if code > 0 {
			return stdout.Bytes(), nil
		}
	}

	msg := stderr.String()
	if msg == "" {
		msg = err.Error()
	}
	return nil, fmt.Errorf("git merge-file failed: %s", msg)
}

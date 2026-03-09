package mergeview

import (
	"context"
	"fmt"
	"os"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/markers"
)

func LoadCanonicalDocument(ctx context.Context, opts cli.Options) (markers.Document, error) {
	_ = ctx
	baseBytes, err := os.ReadFile(opts.BasePath)
	if err != nil {
		return markers.Document{}, fmt.Errorf("read base: %w", err)
	}
	oursBytes, err := os.ReadFile(opts.LocalPath)
	if err != nil {
		return markers.Document{}, fmt.Errorf("read local: %w", err)
	}
	theirsBytes, err := os.ReadFile(opts.RemotePath)
	if err != nil {
		return markers.Document{}, fmt.Errorf("read remote: %w", err)
	}
	return buildSessionFromInputs(baseBytes, oursBytes, theirsBytes).Document(), nil
}

func LoadCanonicalSession(ctx context.Context, opts cli.Options) (*Session, error) {
	_ = ctx
	baseBytes, err := os.ReadFile(opts.BasePath)
	if err != nil {
		return nil, err
	}
	oursBytes, err := os.ReadFile(opts.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("read local: %w", err)
	}
	theirsBytes, err := os.ReadFile(opts.RemotePath)
	if err != nil {
		return nil, fmt.Errorf("read remote: %w", err)
	}
	return buildSessionFromInputs(baseBytes, oursBytes, theirsBytes), nil
}

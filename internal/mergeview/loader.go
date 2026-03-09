package mergeview

import (
	"context"
	"fmt"

	"github.com/chojs23/ec/internal/cli"
	"github.com/chojs23/ec/internal/gitmerge"
	"github.com/chojs23/ec/internal/markers"
)

func LoadCanonicalDocument(ctx context.Context, opts cli.Options) (markers.Document, error) {
	diff3Bytes, err := gitmerge.MergeFileDiff3(ctx, opts.LocalPath, opts.BasePath, opts.RemotePath)
	if err != nil {
		return markers.Document{}, fmt.Errorf("generate diff3 view: %w", err)
	}

	doc, err := markers.Parse(diff3Bytes)
	if err != nil {
		return markers.Document{}, fmt.Errorf("parse diff3 view: %w", err)
	}

	return doc, nil
}

func LoadCanonicalSession(ctx context.Context, opts cli.Options) (*Session, error) {
	doc, err := LoadCanonicalDocument(ctx, opts)
	if err != nil {
		return nil, err
	}
	return SessionFromDocument(doc)
}

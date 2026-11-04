// Package memory reads ordinary files from immutable Git revisions.
package memory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var ErrInvalidPath = errors.New("invalid memory path")

// Repository pins the checkout directory; reads use Git objects, not mutable files.
type Repository struct{ root *os.File }

func Open(root string) (*Repository, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(absolute)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.IsDir() {
		file.Close()
		return nil, errors.New("memory root is not a directory")
	}
	return &Repository{root: file}, nil
}

func (r *Repository) Close() error { return r.root.Close() }

func (r *Repository) Revision(ctx context.Context, requested string) (string, error) {
	explicit := requested != ""
	if requested == "" {
		requested = "HEAD"
	}
	data, err := r.gitOutput(ctx, "rev-parse", "--verify", "--end-of-options", requested+"^{commit}")
	if err != nil {
		return "", err
	}
	revision := strings.TrimSpace(string(data))
	if explicit {
		if _, err := r.gitOutput(ctx, "merge-base", "--is-ancestor", revision, "HEAD"); err != nil {
			return "", errors.New("revision is not published in the read checkout")
		}
	}
	return revision, nil
}

func (r *Repository) MarkdownPaths(ctx context.Context, revision string, categories []string) ([]string, error) {
	data, err := r.gitOutput(ctx, "ls-tree", "-r", "-z", revision, "--", "memories")
	if err != nil {
		return nil, err
	}
	paths := []string{}
	for _, entry := range bytes.Split(data, []byte{0}) {
		meta, name, ok := strings.Cut(string(entry), "\t")
		if !ok || !(strings.HasPrefix(meta, "100644 blob ") || strings.HasPrefix(meta, "100755 blob ")) || path.Ext(name) != ".md" {
			continue
		}
		for _, category := range categories {
			if strings.HasPrefix(name, "memories/"+category+"/") {
				paths = append(paths, name)
				break
			}
		}
	}
	return paths, nil
}

func (r *Repository) Read(ctx context.Context, revision, requested string) ([]byte, error) {
	if requested == "" || path.IsAbs(requested) || path.Clean(requested) != requested || strings.Contains(requested, "\\") || !strings.HasPrefix(requested, "memories/") || path.Ext(requested) != ".md" {
		return nil, ErrInvalidPath
	}
	data, err := r.gitOutput(ctx, "ls-tree", "-z", revision, "--", requested)
	if err != nil {
		return nil, err
	}
	meta, name, ok := strings.Cut(string(bytes.TrimSuffix(data, []byte{0})), "\t")
	fields := strings.Fields(meta)
	if !ok || name != requested || len(fields) != 3 || fields[1] != "blob" || !(fields[0] == "100644" || fields[0] == "100755") {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPath, requested)
	}
	return r.gitOutput(ctx, "cat-file", "blob", fields[2])
}

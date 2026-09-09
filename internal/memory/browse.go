package memory

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
)

func (repository *Repository) MarkdownPaths(ctx context.Context) ([]string, error) {
	var paths []string
	err := repository.withLock(ctx, false, func() error {
		root, err := os.OpenRoot(fmt.Sprintf("/proc/self/fd/%d", repository.root.Fd()))
		if err != nil {
			return err
		}
		defer root.Close()
		return fs.WalkDir(root.FS(), ".", func(filePath string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				if filePath == "." || filePath == "memories" {
					return nil
				}
				return fs.SkipDir
			}
			if entry.Type().IsRegular() && path.Dir(filePath) == "memories" && path.Ext(filePath) == ".md" {
				paths = append(paths, filePath)
			}
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("list memory Markdown: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

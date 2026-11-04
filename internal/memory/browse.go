package memory

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

func (repository *Repository) MarkdownPaths(ctx context.Context, categories []string) ([]string, error) {
	categoryRoots := make(map[string]struct{}, len(categories))
	for _, category := range categories {
		categoryRoots[path.Join("memories", category)] = struct{}{}
	}
	var paths []string
	err := repository.withLock(ctx, func() error {
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
				for categoryRoot := range categoryRoots {
					if filePath == categoryRoot || strings.HasPrefix(filePath, categoryRoot+"/") || strings.HasPrefix(categoryRoot, filePath+"/") {
						return nil
					}
				}
				return fs.SkipDir
			}
			if entry.Type().IsRegular() && path.Ext(filePath) == ".md" {
				for categoryRoot := range categoryRoots {
					if strings.HasPrefix(filePath, categoryRoot+"/") {
						paths = append(paths, filePath)
						break
					}
				}
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

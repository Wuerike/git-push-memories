package consolidation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/unifieddiff"
)

type Checked struct {
	Paths []string `json:"paths"`
	Diff  string   `json:"diff"`
}

// Check prepares IDs before review, then describes the current files, not a preview.
func (m *Manager) Check(ctx context.Context) (Checked, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.changes(ctx, true)
}

func (m *Manager) changes(ctx context.Context, prepare bool) (Checked, error) {
	if m.current == nil {
		return Checked{}, ErrNotPrepared
	}
	p := m.current
	canonical := m.cfg
	canonical.MemoryCategories = p.Categories
	paths := make(map[string]bool, len(p.MemoryPaths))
	baselinePaths := make(map[string]bool, len(p.MemoryPaths))
	for _, path := range p.MemoryPaths {
		paths[path], baselinePaths[path] = true, true
	}
	err := filepath.WalkDir(filepath.Join(p.Worktree, "memories"), func(full string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() || filepath.Ext(full) != ".md" {
			return nil
		}
		relative, err := filepath.Rel(p.Worktree, full)
		if err != nil {
			return err
		}
		path := filepath.ToSlash(relative)
		if !canonical.IsMemoryPath(path) || !entry.Type().IsRegular() {
			return fmt.Errorf("%w: %s", memory.ErrInvalidPath, path)
		}
		paths[path] = true
		return nil
	})
	if err != nil {
		return Checked{}, err
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	result := Checked{Paths: []string{}}
	var diff strings.Builder
	for _, path := range ordered {
		if err := ctx.Err(); err != nil {
			return Checked{}, err
		}
		var baseline []byte
		if baselinePaths[path] {
			baseline, err = m.ws.Repo().Bytes(ctx, "show", p.MainSHA+":"+path)
			if err != nil {
				return Checked{}, err
			}
		}
		full := filepath.Join(p.Worktree, filepath.FromSlash(path))
		current, readErr := os.ReadFile(full)
		if errors.Is(readErr, os.ErrNotExist) {
			if baselinePaths[path] {
				result.Paths = append(result.Paths, path)
				diff.WriteString(unifieddiff.Delete(path, baseline))
			}
			continue
		}
		if readErr != nil {
			return Checked{}, readErr
		}
		if baselinePaths[path] && bytes.Equal(current, baseline) {
			continue
		}
		normalized, err := memory.PrepareDocument(path, baseline, current)
		if err != nil {
			return Checked{}, fmt.Errorf("%s: %w", path, err)
		}
		if !bytes.Equal(current, normalized) {
			if !prepare {
				return Checked{}, fmt.Errorf("%s needs a new check and review before publication", path)
			}
			if err := os.WriteFile(full, normalized, 0o644); err != nil {
				return Checked{}, err
			}
		}
		if baselinePaths[path] && bytes.Equal(normalized, baseline) {
			continue
		}
		result.Paths = append(result.Paths, path)
		if baselinePaths[path] {
			diff.WriteString(unifieddiff.Between(path, baseline, normalized))
		} else {
			diff.WriteString(unifieddiff.Create(path, normalized))
		}
	}
	result.Diff = diff.String()
	return result, nil
}

package consolidation

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/Wuerike/git-push-memories/internal/workspace"
)

type Checked struct {
	Paths []string `json:"paths"`
	Diff  string   `json:"diff"`
}

// Check observes current files using a temporary Git index outside the worktree.
func (m *Manager) Check(ctx context.Context) (Checked, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.changes(ctx)
}

func (m *Manager) changes(ctx context.Context) (Checked, error) {
	if m.current == nil {
		return Checked{}, ErrNotPrepared
	}
	p := m.current
	index, err := os.CreateTemp(m.cfg.StateRoot, "review-index-*")
	if err != nil {
		return Checked{}, err
	}
	name := index.Name()
	index.Close()
	os.Remove(name)
	defer os.Remove(name)
	git := workspace.Git{Dir: p.Worktree, IndexFile: name}
	if _, err := git.Output(ctx, "read-tree", p.MainSHA); err != nil {
		return Checked{}, err
	}
	candidates, err := git.Bytes(ctx, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", "memories")
	if err != nil {
		return Checked{}, err
	}
	if len(candidates) == 0 {
		return Checked{Paths: []string{}}, nil
	}
	rawPaths := []string{}
	seen := map[string]bool{}
	for _, encoded := range bytes.Split(candidates, []byte{0}) {
		name := string(encoded)
		if name != "" && filepath.Ext(name) == ".md" && !seen[name] {
			rawPaths = append(rawPaths, name)
			seen[name] = true
		}
	}
	if err := git.StageFiles(ctx, rawPaths...); err != nil {
		return Checked{}, err
	}
	changed, err := git.Bytes(ctx, "diff", "--cached", "--no-renames", "--name-only", "-z", p.MainSHA, "--")
	if err != nil {
		return Checked{}, err
	}
	paths := []string{}
	for _, encoded := range bytes.Split(changed, []byte{0}) {
		name := string(encoded)
		if name == "" || filepath.Ext(name) != ".md" {
			continue
		}

		paths = append(paths, name)
	}
	if len(paths) == 0 {
		return Checked{Paths: paths}, nil
	}
	arguments := append([]string{"diff", "--cached", "--no-renames", "--no-ext-diff", "--no-textconv", "--no-color", p.MainSHA, "--"}, paths...)
	diff, err := git.Bytes(ctx, arguments...)
	if err != nil {
		return Checked{}, err
	}
	return Checked{Paths: paths, Diff: string(diff)}, nil
}

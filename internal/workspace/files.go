package workspace

import (
	"context"
	"fmt"
	"os"
)

// StageFiles records exact file bytes in this Git index, bypassing Git's text
// conversion and clean filters. Callers supply regular files within the worktree.
func (git Git) StageFiles(ctx context.Context, paths ...string) error {
	root, err := os.OpenRoot(git.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range paths {
		info, err := root.Lstat(name)
		if os.IsNotExist(err) {
			if _, err := git.Output(ctx, "update-index", "--force-remove", "--", name); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("memory path is not a regular file: %s", name)
		}
		mode := "100644"
		if info.Mode().Perm()&0o111 != 0 {
			mode = "100755"
		}
		contents, err := root.ReadFile(name)
		if err != nil {
			return err
		}
		blob, err := git.run(ctx, contents, "hash-object", "-w", "--no-filters", "--stdin")
		if err != nil {
			return err
		}
		if _, err := git.Output(ctx, "update-index", "--add", "--cacheinfo", mode, blob, name); err != nil {
			return err
		}
	}
	return nil
}

// CommitFiles publishes selected raw blobs using an isolated index. Unrelated
// staging in the worktree index is retained; only committed paths are reset.
func (w *Workspace) CommitFiles(ctx context.Context, dir, message string, paths ...string) error {
	index, err := os.CreateTemp(w.Paths.State, "commit-index-*")
	if err != nil {
		return err
	}
	name := index.Name()
	index.Close()
	os.Remove(name)
	defer os.Remove(name)
	git := Git{Dir: dir, IndexFile: name}
	if _, err := git.Output(ctx, "read-tree", "HEAD"); err != nil {
		return err
	}
	if err := git.StageFiles(ctx, paths...); err != nil {
		return err
	}
	identity, err := w.identity(ctx)
	if err != nil {
		return err
	}
	arguments := append(identity, "commit", "--no-verify", "--no-edit", "-m", message)
	if _, err := git.Output(ctx, arguments...); err != nil {
		return err
	}
	_, err = (Git{Dir: dir}).Output(ctx, append([]string{"reset", "-q", "HEAD", "--"}, paths...)...)
	return err
}

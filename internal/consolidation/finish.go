package consolidation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Wuerike/git-push-memories/internal/forge/github"
	"github.com/Wuerike/git-push-memories/internal/repolock"
	"github.com/Wuerike/git-push-memories/internal/workspace"
)

type Finished struct {
	Found     bool     `json:"found"`
	Merged    bool     `json:"merged"`
	PRURL     string   `json:"pr_url,omitempty"`
	Removed   []string `json:"removed"`
	Preserved []string `json:"preserved"`
}

// Finish confirms a published PR and removes only unchanged, concluded inputs.
// It does not discover or restore a previous editing session.
func (m *Manager) Finish(ctx context.Context) (Finished, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := Finished{Removed: []string{}, Preserved: []string{}}
	data, err := os.ReadFile(m.cfg.Paths.Published)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return Finished{}, err
	}
	var record Published
	if err := json.Unmarshal(data, &record); err != nil {
		return Finished{}, fmt.Errorf("read publication record: %w", err)
	}
	result.Found, result.PRURL = true, record.PRURL
	client, err := github.New(m.cfg.RepoURL)
	if err != nil {
		return Finished{}, err
	}
	if err := client.AuthStatus(ctx); err != nil {
		return Finished{}, err
	}
	pr, err := client.PRView(ctx, record.PRNumber)
	if err != nil {
		return Finished{}, err
	}
	if pr.State == "CLOSED" {
		return Finished{}, fmt.Errorf("PR %s was closed without merging; its proposals were kept", record.PRURL)
	}
	if pr.State != "MERGED" {
		return result, nil
	}
	if err := m.ws.Fetch(ctx); err != nil {
		return Finished{}, err
	}
	if pr.MergeCommit.OID == "" || !m.ws.Repo().Ok(ctx, "merge-base", "--is-ancestor", pr.MergeCommit.OID, "origin/main") {
		return Finished{}, errors.New("the merged PR commit is not present in origin/main")
	}
	latest, err := workspace.Rev(ctx, m.cfg.Paths.Repo, "origin/proposals")
	if err != nil {
		return Finished{}, err
	}
	entries, err := m.entries(ctx, latest, "proposals")
	if err != nil {
		return Finished{}, err
	}
	blobs := make(map[string]string, len(entries))
	for _, entry := range entries {
		if (entry.mode == "100644" || entry.mode == "100755") && entry.kind == "blob" {
			blobs[entry.path] = entry.blob
		}
	}
	err = repolock.With(ctx, m.cfg.StateRoot, func() error {
		dir := m.cfg.Paths.Proposals
		git := workspace.Git{Dir: dir}
		branch, err := git.Output(ctx, "branch", "--show-current")
		if err != nil || branch != workspace.ProposalsBranch {
			return errors.New("the local inbox is not on the proposals branch")
		}
		status, err := git.Output(ctx, "status", "--porcelain")
		if err != nil {
			return err
		}
		if status != "" {
			return workspace.ErrDirty
		}
		if _, err := git.Output(ctx, "merge", "--ff-only", latest); err != nil {
			return err
		}
		if head, err := workspace.Rev(ctx, dir, "HEAD"); err != nil || head != latest {
			return errors.New("the local inbox has unpublished commits; resolve them before finishing")
		}
		for _, proposal := range record.Proposals {
			if blob, exists := blobs[proposal.Path]; !exists || blob != proposal.Blob {
				result.Preserved = append(result.Preserved, proposal.Path)
				continue
			}
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(proposal.Path))); err != nil {
				return err
			}
			result.Removed = append(result.Removed, proposal.Path)
		}
		if len(result.Removed) == 0 {
			return nil
		}
		if err := m.ws.Commit(ctx, dir, "memory: remove concluded proposals", result.Removed...); err != nil {
			return err
		}
		return m.ws.PushCAS(ctx, dir, workspace.ProposalsBranch)
	})
	if err != nil {
		return Finished{}, err
	}
	if err := m.ws.UpdateRead(ctx); err != nil {
		return Finished{}, err
	}
	if err := os.Remove(m.cfg.Paths.Published); err != nil {
		return Finished{}, err
	}
	result.Merged = true
	return result, nil
}

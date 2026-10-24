package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Wuerike/git-push-memories/internal/forge/github"
	"github.com/Wuerike/git-push-memories/internal/workspace"
)

var (
	ErrPending        = errors.New("proposals are still pending in the release")
	ErrMerged         = errors.New("the release pull request is already merged; use --finish")
	ErrClosedUnmerged = errors.New("the release pull request was closed without merging; use --resume to open another or --abort")
	ErrNotInMain      = errors.New("the release pull request is merged but its commit has not reached origin/main yet")
)

// Outcome tells what Finalize found.
type Outcome int

const (
	NotMerged Outcome = iota
	Finalized
)

func (m *Manager) forge(ctx context.Context) (github.Client, error) {
	client, err := github.New(m.cfg.RepoURL)
	if err != nil {
		return github.Client{}, err
	}
	return client, client.AuthStatus(ctx)
}

// Submit pushes the release branch and opens its pull request, or reuses or finalizes
// according to the state of the branch's latest PR. It returns the PR URL ("" when the release was finalized).
func (m *Manager) Submit(ctx context.Context) (url string, err error) {
	err = m.locked(ctx, func() error {
		client, err := m.forge(ctx)
		if err != nil {
			return err
		}
		if err := m.ws.Fetch(ctx); err != nil {
			return err
		}
		marker, found, err := m.remoteMarker(ctx)
		if err != nil {
			return err
		}
		if !found {
			return ErrNoRelease
		}
		pr, err := client.PRFind(ctx, marker.ReleaseBranch)
		if err != nil {
			return err
		}
		if pr != nil && pr.State == "MERGED" {
			_, err := m.finalize(ctx, marker, client, false)
			return err
		}
		root, branch, ok := m.Active(ctx)
		if !ok || branch != marker.ReleaseBranch {
			return ErrNothingToResume
		}
		pending, err := pendingIDs(root)
		if err != nil {
			return err
		}
		if len(pending) > 0 {
			return ErrPending
		}
		if err := workspace.Normalize(ctx, root); err != nil {
			return err
		}
		if m.ws.RefExists(ctx, "refs/remotes/origin/"+branch) {
			if err := m.ws.Merge(ctx, root, "origin/"+branch); err != nil {
				return err
			}
		}
		if err := m.ws.Push(ctx, root, branch); err != nil {
			return err
		}
		if pr != nil && pr.State == "OPEN" {
			url = pr.URL
			return nil
		}
		title := "memory: promote proposals (" + marker.OpenedAt + ")"
		body := fmt.Sprintf("Release %s opened by %s consolidating %d proposal(s) into canonical memory.\n\nReview the memory diff; merging with squash keeps main history clean.", marker.OpenedAt, marker.OpenedBy, len(marker.Proposals))
		url, err = client.PRCreate(ctx, workspace.MainBranch, branch, title, body)
		return err
	})
	return url, err
}

// Finalize closes a release whose pull request was merged: proposals gets the new main and loses the
// consumed proposals and the marker in one push. With force, the person vouches that the PR was merged.
func (m *Manager) Finalize(ctx context.Context, force bool) (Outcome, error) {
	outcome := NotMerged
	err := m.locked(ctx, func() error {
		if err := m.ws.Fetch(ctx); err != nil {
			return err
		}
		marker, found, err := m.remoteMarker(ctx)
		if err != nil {
			return err
		}
		if !found {
			return ErrNoRelease
		}
		var client github.Client
		if !force {
			if client, err = m.forge(ctx); err != nil {
				return err
			}
		}
		outcome, err = m.finalize(ctx, marker, client, force)
		return err
	})
	return outcome, err
}

// finalize runs under the lock with a fetch already done.
func (m *Manager) finalize(ctx context.Context, marker Marker, client github.Client, force bool) (Outcome, error) {
	if !force {
		pr, err := client.PRFind(ctx, marker.ReleaseBranch)
		if err != nil {
			return NotMerged, err
		}
		switch {
		case pr == nil || pr.State == "OPEN":
			return NotMerged, nil
		case pr.State == "CLOSED":
			return NotMerged, ErrClosedUnmerged
		case pr.State != "MERGED":
			return NotMerged, nil
		}
		if pr.MergeCommit.OID == "" || !m.ws.Repo().Ok(ctx, "merge-base", "--is-ancestor", pr.MergeCommit.OID, "refs/remotes/origin/main") {
			return NotMerged, ErrNotInMain
		}
	}
	dir := m.ws.Paths.Proposals
	if err := workspace.Normalize(ctx, dir); err != nil {
		return NotMerged, err
	}
	if err := m.ws.Merge(ctx, dir, "origin/proposals"); err != nil {
		return NotMerged, err
	}
	if err := m.ws.Merge(ctx, dir, "origin/main"); err != nil {
		return NotMerged, err
	}
	var removed []string
	for _, id := range marker.Proposals {
		matches, _ := filepath.Glob(filepath.Join(dir, "proposals", id+"-*.md"))
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				return NotMerged, err
			}
			removed = append(removed, filepath.ToSlash(filepath.Join("proposals", filepath.Base(match))))
		}
	}
	if err := os.Remove(filepath.Join(dir, MarkerFile)); err == nil {
		removed = append(removed, MarkerFile)
	}
	if len(removed) > 0 {
		if err := m.ws.Commit(ctx, dir, "chore: finalize release "+marker.OpenedAt, removed...); err != nil {
			return NotMerged, err
		}
	}
	// Push even without a new commit: an earlier interrupted run may have left it unsent.
	if err := m.ws.Push(ctx, dir, workspace.ProposalsBranch); err != nil {
		return NotMerged, err
	}
	_ = m.ws.DeleteRemoteBranch(ctx, marker.ReleaseBranch)
	m.cleanOrphans(ctx, "")
	_ = m.ws.UpdateReadLocked(ctx)
	return Finalized, nil
}

// finalizeIfMerged is the Open-time trigger: a merged release is closed before a new one opens.
func (m *Manager) finalizeIfMerged(ctx context.Context, marker Marker) (bool, error) {
	client, err := m.forge(ctx)
	if err != nil {
		return false, fmt.Errorf("%w: opened by %s at %s; gh cannot confirm whether it was merged (%v): run `gh auth login` or `gpm promote --finish`", ErrActive, marker.OpenedBy, marker.OpenedAt, err)
	}
	outcome, err := m.finalize(ctx, marker, client, false)
	if err != nil && !errors.Is(err, ErrClosedUnmerged) {
		return false, err
	}
	return outcome == Finalized, nil
}

// checkNotMerged refuses to abort a release whose PR was already merged.
func (m *Manager) checkNotMerged(ctx context.Context, marker Marker) error {
	if !m.ws.RefExists(ctx, "refs/remotes/origin/"+marker.ReleaseBranch) {
		return nil // never sent for review, so there is no PR to lose
	}
	client, err := m.forge(ctx)
	if err != nil {
		return fmt.Errorf("cannot confirm that the release PR is not merged: %w", err)
	}
	pr, err := client.PRFind(ctx, marker.ReleaseBranch)
	if err != nil {
		return err
	}
	if pr != nil && pr.State == "MERGED" {
		return ErrMerged
	}
	return nil
}

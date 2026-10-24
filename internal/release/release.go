// Package release runs the /promote lifecycle on top of the workspace: it opens a
// release (the marker pushed to proposals is the lock), exposes the release worktree
// where proposals are consolidated, and aborts or resumes it.
package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/repolock"
	"github.com/Wuerike/git-push-memories/internal/workspace"
)

const (
	MarkerFile    = "release.lock.json"
	markerVersion = 1
	maxAttempts   = 5
)

var (
	ErrActive          = errors.New("a release is already in progress")
	ErrNoRelease       = errors.New("there is no release in progress")
	ErrNoProposals     = errors.New("there are no proposals to promote")
	ErrNothingToResume = errors.New("the release branch is not available on this machine or on the remote; only --abort is possible")
)

var (
	openedAtPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z$`)
	objectPattern   = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	idPattern       = regexp.MustCompile(`^[0-9a-f]{32}$`)
	proposalFile    = regexp.MustCompile(`^([0-9a-f]{32})-[a-z0-9]+(?:-[a-z0-9]+)*\.md$`)
)

// Marker is release.lock.json, kept on the proposals branch while a release is in progress.
type Marker struct {
	SchemaVersion int      `json:"schema_version"`
	ReleaseBranch string   `json:"release_branch"`
	Base          string   `json:"base"`
	Proposals     []string `json:"proposals"`
	OpenedBy      string   `json:"opened_by"`
	OpenedAt      string   `json:"opened_at"`
}

func (marker Marker) validate() error {
	if marker.SchemaVersion != markerVersion || !openedAtPattern.MatchString(marker.OpenedAt) ||
		marker.ReleaseBranch != workspace.ReleasePrefix+marker.OpenedAt || !objectPattern.MatchString(marker.Base) || marker.OpenedBy == "" {
		return errors.New("release marker is invalid")
	}
	for _, id := range marker.Proposals {
		if !idPattern.MatchString(id) {
			return errors.New("release marker is invalid")
		}
	}
	return nil
}

// Status describes the release in progress, if any.
type Status struct {
	Open         bool     `json:"open"`
	Marker       *Marker  `json:"marker,omitempty"`
	Local        bool     `json:"local"`             // the release worktree exists on this machine
	Pending      []string `json:"pending,omitempty"` // IDs still in proposals/ on the release
	PendingKnown bool     `json:"pending_known"`     // false when the release branch is not available here
	ReleaseRoot  string   `json:"-"`
}

type Manager struct {
	cfg config.Config
	ws  *workspace.Workspace
}

func New(cfg config.Config) *Manager {
	return &Manager{cfg: cfg, ws: cfg.Workspace()}
}

func (m *Manager) locked(ctx context.Context, action func() error) error {
	return repolock.With(ctx, m.cfg.StateRoot, action)
}

// Active returns the release worktree root and branch when a release is checked out on this machine.
func (m *Manager) Active(ctx context.Context) (root, branch string, ok bool) {
	branch = m.ws.ReleaseBranch(ctx)
	if branch == "" || !strings.HasPrefix(branch, workspace.ReleasePrefix) {
		return "", "", false
	}
	return m.ws.Paths.Release, branch, true
}

func (m *Manager) remoteMarker(ctx context.Context) (Marker, bool, error) {
	const ref = "refs/remotes/origin/proposals"
	if !m.ws.RefExists(ctx, ref) || !m.ws.Repo().Ok(ctx, "cat-file", "-e", ref+":"+MarkerFile) {
		return Marker{}, false, nil
	}
	contents, err := m.ws.ShowFile(ctx, ref, MarkerFile)
	if err != nil {
		return Marker{}, false, err
	}
	var marker Marker
	if err := json.Unmarshal(contents, &marker); err != nil {
		return Marker{}, false, errors.New("release marker is invalid")
	}
	if err := marker.validate(); err != nil {
		return Marker{}, false, err
	}
	return marker, true, nil
}

func pendingIDs(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "proposals"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if match := proposalFile.FindStringSubmatch(entry.Name()); match != nil {
			ids = append(ids, match[1])
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// Status reads the marker from the last fetched proposals branch, fetching first when the network allows.
func (m *Manager) Status(ctx context.Context) (Status, error) {
	_ = m.ws.Fetch(ctx) // offline: report what was last seen
	return m.status(ctx)
}

func (m *Manager) status(ctx context.Context) (Status, error) {
	marker, found, err := m.remoteMarker(ctx)
	if err != nil || !found {
		return Status{}, err
	}
	status := Status{Open: true, Marker: &marker}
	switch root, branch, ok := m.Active(ctx); {
	case ok && branch == marker.ReleaseBranch:
		status.Local, status.ReleaseRoot, status.PendingKnown = true, root, true
		status.Pending, err = pendingIDs(root)
	case m.ws.RefExists(ctx, "refs/remotes/origin/"+marker.ReleaseBranch):
		status.PendingKnown = true
		status.Pending, err = m.remotePending(ctx, marker.ReleaseBranch)
	}
	return status, err
}

func (m *Manager) remotePending(ctx context.Context, branch string) ([]string, error) {
	out, err := m.ws.Repo().Output(ctx, "ls-tree", "--name-only", "refs/remotes/origin/"+branch, "proposals/")
	if err != nil || out == "" {
		return nil, err
	}
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		if match := proposalFile.FindStringSubmatch(filepath.Base(line)); match != nil {
			ids = append(ids, match[1])
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// Open takes the lock by pushing the marker to proposals, then cuts the local release branch at base.
func (m *Manager) Open(ctx context.Context) (Status, error) {
	var status Status
	err := m.locked(ctx, func() error {
		var err error
		status, err = m.open(ctx)
		return err
	})
	return status, err
}

func (m *Manager) open(ctx context.Context) (Status, error) {
	dir := m.ws.Paths.Proposals
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := m.ws.Fetch(ctx); err != nil {
			return Status{}, err
		}
		if marker, found, err := m.remoteMarker(ctx); err != nil {
			return Status{}, err
		} else if found {
			finalized, err := m.finalizeIfMerged(ctx, marker)
			if err != nil {
				return Status{}, err
			}
			if finalized {
				continue
			}
			return Status{}, fmt.Errorf("%w: opened by %s at %s", ErrActive, marker.OpenedBy, marker.OpenedAt)
		}
		m.cleanOrphans(ctx, "")
		if err := workspace.Normalize(ctx, dir); err != nil {
			return Status{}, err
		}
		if m.ws.RefExists(ctx, "refs/remotes/origin/proposals") {
			if err := m.ws.Merge(ctx, dir, "origin/proposals"); err != nil {
				return Status{}, err
			}
		}
		head, err := workspace.Rev(ctx, dir, "HEAD")
		if err != nil {
			return Status{}, err
		}
		if err := m.ws.Merge(ctx, dir, "origin/main"); err != nil {
			return Status{}, err
		}
		ids, err := pendingIDs(dir)
		if err != nil {
			return Status{}, err
		}
		if len(ids) == 0 {
			_ = workspace.ResetHard(ctx, dir, head)
			return Status{}, ErrNoProposals
		}
		base, err := workspace.Rev(ctx, dir, "HEAD")
		if err != nil {
			return Status{}, err
		}
		openedBy, err := m.ws.Repo().Output(ctx, "config", "user.email")
		if err != nil || openedBy == "" {
			return Status{}, errors.New("Git identity is required: set user.name and user.email")
		}
		openedAt := time.Now().UTC().Format("20060102T150405Z")
		marker := Marker{SchemaVersion: markerVersion, ReleaseBranch: workspace.ReleasePrefix + openedAt, Base: base, Proposals: ids, OpenedBy: openedBy, OpenedAt: openedAt}
		payload, err := json.MarshalIndent(marker, "", "  ")
		if err != nil {
			return Status{}, err
		}
		if err := os.WriteFile(filepath.Join(dir, MarkerFile), append(payload, '\n'), 0o644); err != nil {
			return Status{}, errors.New("release marker could not be written")
		}
		err = m.ws.Commit(ctx, dir, "chore: open release "+openedAt, MarkerFile)
		if err == nil {
			err = m.ws.PushCAS(ctx, dir, workspace.ProposalsBranch)
		}
		if err != nil {
			// Drop only the merge and the marker commit; unsent proposals made before head stay.
			if resetErr := workspace.ResetHard(ctx, dir, head); resetErr != nil {
				return Status{}, errors.Join(err, resetErr)
			}
			if errors.Is(err, workspace.ErrRejected) {
				continue
			}
			return Status{}, err
		}
		if err := m.ws.AddReleaseWorktree(ctx, marker.ReleaseBranch, base, true); err != nil {
			return Status{}, fmt.Errorf("release is locked but its worktree could not be created (use --resume): %w", err)
		}
		return m.status(ctx)
	}
	return Status{}, workspace.ErrRejected
}

// OpenOrResume continues the release in progress, closing it first when its PR was merged, or opens a new one.
func (m *Manager) OpenOrResume(ctx context.Context) (Status, error) {
	status, err := m.Status(ctx)
	if err != nil {
		return Status{}, err
	}
	if !status.Open {
		return m.Open(ctx)
	}
	var finalized bool
	if err := m.locked(ctx, func() error {
		var err error
		finalized, err = m.finalizeIfMerged(ctx, *status.Marker)
		return err
	}); err != nil {
		return Status{}, err
	}
	if finalized {
		return m.Open(ctx)
	}
	return m.Resume(ctx)
}

// Resume makes the release worktree available on this machine and reports its status.
func (m *Manager) Resume(ctx context.Context) (Status, error) {
	var status Status
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
		m.cleanOrphans(ctx, marker.ReleaseBranch)
		if m.ws.ReleaseBranch(ctx) != marker.ReleaseBranch {
			branches, err := m.ws.LocalBranches(ctx, marker.ReleaseBranch)
			if err != nil {
				return err
			}
			switch {
			case slices.Contains(branches, marker.ReleaseBranch):
				err = m.ws.AddReleaseWorktree(ctx, marker.ReleaseBranch, "", false)
			case m.ws.RefExists(ctx, "refs/remotes/origin/"+marker.ReleaseBranch):
				err = m.ws.AddReleaseWorktree(ctx, marker.ReleaseBranch, "origin/"+marker.ReleaseBranch, true)
			default:
				err = ErrNothingToResume
			}
			if err != nil {
				return err
			}
		}
		status, err = m.status(ctx)
		return err
	})
	return status, err
}

// Abort drops the lock and the release branch. Proposals stay untouched on proposals.
func (m *Manager) Abort(ctx context.Context) error {
	return m.locked(ctx, func() error {
		if err := m.ws.Fetch(ctx); err != nil {
			return err
		}
		marker, found, err := m.remoteMarker(ctx)
		if err != nil {
			return err
		}
		if found {
			if err := m.checkNotMerged(ctx, marker); err != nil {
				return err
			}
			if err := m.dropMarker(ctx); err != nil {
				return err
			}
			_ = m.ws.DeleteRemoteBranch(ctx, marker.ReleaseBranch)
		}
		m.cleanOrphans(ctx, "")
		return nil
	})
}

// dropMarker removes the marker from proposals and pushes, which releases the lock.
func (m *Manager) dropMarker(ctx context.Context) error {
	dir := m.ws.Paths.Proposals
	if err := workspace.Normalize(ctx, dir); err != nil {
		return err
	}
	if err := m.ws.Merge(ctx, dir, "origin/proposals"); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, MarkerFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := m.ws.Commit(ctx, dir, "chore: close release", MarkerFile); err != nil {
		return err
	}
	return m.ws.Push(ctx, dir, workspace.ProposalsBranch)
}

// cleanOrphans removes local release branches, the worktree and transaction leftovers that do not belong to keep.
func (m *Manager) cleanOrphans(ctx context.Context, keep string) {
	branches, _ := m.ws.LocalBranches(ctx, workspace.ReleasePrefix)
	current := m.ws.ReleaseBranch(ctx)
	removed := false
	for _, branch := range branches {
		if branch == keep {
			continue
		}
		if branch == current {
			m.ws.RemoveRelease(ctx, branch)
		} else {
			_, _ = m.ws.Repo().Output(ctx, "branch", "-D", branch)
		}
		removed = true
	}
	if current != "" && current != keep && !removed {
		m.ws.RemoveRelease(ctx, "")
		removed = true
	}
	if removed {
		m.cleanTransactionState()
	}
}

func (m *Manager) cleanTransactionState() {
	entries, err := os.ReadDir(m.cfg.StateRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "promotion-transaction.json" || strings.HasPrefix(name, ".promotion-marker-") || strings.HasPrefix(name, ".git-push-memories-") {
			_ = os.Remove(filepath.Join(m.cfg.StateRoot, name))
		}
	}
}

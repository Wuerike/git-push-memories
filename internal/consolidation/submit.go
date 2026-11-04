package consolidation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/Wuerike/git-push-memories/internal/forge/github"
	"github.com/Wuerike/git-push-memories/internal/workspace"
)

var (
	ErrNoChanges = errors.New("there are no canonical changes to publish")
	ErrSummary   = errors.New("a consolidation summary is required")
)

type ProposalBlob struct {
	Path string `json:"path"`
	Blob string `json:"blob"`
}

// Published is a cleanup record, not a saved editing session.
type Published struct {
	PRNumber  int            `json:"pr_number"`
	PRURL     string         `json:"pr_url"`
	Branch    string         `json:"branch"`
	Proposals []ProposalBlob `json:"proposals"`
}

type Submitted struct {
	Commit   string `json:"commit"`
	PRNumber int    `json:"pr_number"`
	PRURL    string `json:"pr_url"`
}

// Submit commits the current files. The caller conducts approval of their diff;
// this method never reapplies a stored preview or changes the reviewed prose.
func (m *Manager) Submit(ctx context.Context, summary string) (Submitted, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return Submitted{}, ErrNotPrepared
	}
	if strings.TrimSpace(summary) == "" {
		return Submitted{}, ErrSummary
	}
	if _, err := os.Stat(m.cfg.Paths.Published); err == nil {
		return Submitted{}, errors.New("finish the published promotion before sending another")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Submitted{}, err
	}
	checked, err := m.changes(ctx, false)
	if err != nil {
		return Submitted{}, err
	}
	if len(checked.Paths) == 0 {
		return Submitted{}, ErrNoChanges
	}
	client, err := github.New(m.cfg.RepoURL)
	if err != nil {
		return Submitted{}, err
	}
	if err := client.AuthStatus(ctx); err != nil {
		return Submitted{}, err
	}
	p := m.current
	if err := m.ws.Commit(ctx, p.Worktree, "memory: consolidate proposals", checked.Paths...); err != nil {
		return Submitted{}, err
	}
	commit, err := workspace.Rev(ctx, p.Worktree, "HEAD")
	if err != nil {
		return Submitted{}, err
	}
	if err := m.ws.PushCAS(ctx, p.Worktree, p.Branch); err != nil {
		return Submitted{}, err
	}
	url, err := client.PRCreate(ctx, "main", p.Branch, "memory: consolidate proposals", summary)
	if err != nil {
		return Submitted{}, err
	}
	number, err := strconv.Atoi(path.Base(strings.TrimSuffix(url, "/")))
	if err != nil || number <= 0 {
		return Submitted{}, fmt.Errorf("PR created at %s but its number could not be read", url)
	}
	record := Published{PRNumber: number, PRURL: url, Branch: p.Branch, Proposals: make([]ProposalBlob, len(p.Proposals))}
	for i, proposal := range p.Proposals {
		record.Proposals[i] = ProposalBlob{Path: proposal.Path, Blob: proposal.Blob}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Submitted{}, err
	}
	if err := os.WriteFile(m.cfg.Paths.Published, append(data, '\n'), 0o600); err != nil {
		return Submitted{}, fmt.Errorf("PR created at %s but its cleanup record could not be saved: %w", url, err)
	}
	m.current = nil
	return Submitted{Commit: commit, PRNumber: number, PRURL: url}, nil
}

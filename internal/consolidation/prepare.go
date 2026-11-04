// Package consolidation supports an agent-led promotion over ordinary files.
// It manages Git and canonical formatting, never content decisions per proposal.
package consolidation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/workspace"
)

var (
	ErrNoProposals = errors.New("there are no proposals to promote")
	ErrNotPrepared = errors.New("prepare a promotion first")
)

var proposalName = regexp.MustCompile(`^proposals/([0-9a-f]{32})-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$`)

type ProposalInput struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Title     string `json:"title"`
	Blob      string `json:"blob"`
	InputPath string `json:"input_path"`
}

type Prepared struct {
	Worktree     string          `json:"worktree"`
	InputsRoot   string          `json:"inputs_root"`
	Branch       string          `json:"branch"`
	MainSHA      string          `json:"main_sha"`
	ProposalsSHA string          `json:"proposals_sha"`
	Categories   []string        `json:"categories"`
	MemoryPaths  []string        `json:"memory_paths"`
	Proposals    []ProposalInput `json:"proposals"`
}

type Manager struct {
	cfg     config.Config
	ws      *workspace.Workspace
	mu      sync.Mutex
	current *Prepared // current process only; never serialized for resumption
}

func New(cfg config.Config) *Manager { return &Manager{cfg: cfg, ws: cfg.Workspace()} }

type treeEntry struct{ mode, kind, blob, path string }

func (m *Manager) entries(ctx context.Context, revision, root string) ([]treeEntry, error) {
	data, err := m.ws.Repo().Bytes(ctx, "ls-tree", "-r", "-z", "--full-tree", revision, "--", root)
	if err != nil {
		return nil, err
	}
	var entries []treeEntry
	for _, encoded := range bytes.Split(data, []byte{0}) {
		if len(encoded) == 0 {
			continue
		}
		meta, name, ok := strings.Cut(string(encoded), "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return nil, errors.New("invalid Git tree entry")
		}
		entries = append(entries, treeEntry{fields[0], fields[1], fields[2], name})
	}
	return entries, nil
}

func (m *Manager) Prepare(ctx context.Context) (Prepared, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != nil {
		return Prepared{}, errors.New("a promotion is already prepared in this process")
	}
	if err := m.ws.Fetch(ctx); err != nil {
		return Prepared{}, err
	}
	mainSHA, err := workspace.Rev(ctx, m.cfg.Paths.Repo, "origin/main")
	if err != nil {
		return Prepared{}, err
	}
	proposalSHA, err := workspace.Rev(ctx, m.cfg.Paths.Repo, "origin/proposals")
	if err != nil {
		return Prepared{}, err
	}
	settings, err := m.ws.ShowFile(ctx, mainSHA, "gpm.json")
	if err != nil {
		return Prepared{}, err
	}
	categories, err := config.ParseRepositoryConfig(settings)
	if err != nil {
		return Prepared{}, err
	}
	canonical := m.cfg
	canonical.MemoryCategories = categories
	proposalEntries, err := m.entries(ctx, proposalSHA, "proposals")
	if err != nil {
		return Prepared{}, err
	}
	var proposals []ProposalInput
	contents := make(map[string][]byte)
	for _, entry := range proposalEntries {
		match := proposalName.FindStringSubmatch(entry.path)
		if match == nil || entry.mode != "100644" || entry.kind != "blob" {
			continue
		}
		data, err := m.ws.Repo().Bytes(ctx, "cat-file", "blob", entry.blob)
		if err != nil {
			return Prepared{}, fmt.Errorf("read proposal %s: %w", entry.path, err)
		}
		title := match[2]
		for _, line := range strings.Split(string(data), "\n") {
			if heading, ok := strings.CutPrefix(line, "# "); ok && strings.TrimSpace(heading) != "" {
				title = strings.TrimSpace(heading)
				break
			}
		}
		proposals = append(proposals, ProposalInput{ID: match[1], Path: entry.path, Title: title, Blob: entry.blob})
		contents[entry.path] = data
	}
	if len(proposals) == 0 {
		return Prepared{}, ErrNoProposals
	}
	memories, err := m.entries(ctx, mainSHA, "memories")
	if err != nil {
		return Prepared{}, err
	}
	memoryPaths := []string{}
	for _, entry := range memories {
		if canonical.IsMemoryPath(entry.path) && entry.mode == "100644" && entry.kind == "blob" {
			memoryPaths = append(memoryPaths, entry.path)
		}
	}
	sort.Strings(memoryPaths)
	paths, err := m.ws.NewPromotion(ctx, mainSHA)
	if err != nil {
		return Prepared{}, err
	}
	for i := range proposals {
		proposal := &proposals[i]
		proposal.InputPath = filepath.Join(paths.InputsRoot, filepath.FromSlash(proposal.Path))
		if err := os.MkdirAll(filepath.Dir(proposal.InputPath), 0o700); err != nil {
			return Prepared{}, err
		}
		if err := os.WriteFile(proposal.InputPath, contents[proposal.Path], 0o600); err != nil {
			return Prepared{}, err
		}
	}
	prepared := Prepared{Worktree: paths.Worktree, InputsRoot: paths.InputsRoot, Branch: paths.Branch,
		MainSHA: mainSHA, ProposalsSHA: proposalSHA, Categories: categories, MemoryPaths: memoryPaths, Proposals: proposals}
	m.current = &prepared
	return prepared, nil
}

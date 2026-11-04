// Package consolidation supports an agent-led promotion over ordinary files.
// It manages Git and files; the agent makes content decisions.
package consolidation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

type ProposalInput struct {
	Path      string `json:"path"`
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

func (m *Manager) Prepare(ctx context.Context, selected ...string) (Prepared, error) {
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
	wanted := make(map[string]bool, len(selected))
	for _, name := range selected {
		wanted[name] = false
	}
	var proposals []ProposalInput
	contents := make(map[string][]byte)
	for _, entry := range proposalEntries {
		if !strings.HasSuffix(entry.path, ".md") || !(entry.mode == "100644" || entry.mode == "100755") || entry.kind != "blob" {
			continue
		}
		if len(selected) > 0 {
			if _, ok := wanted[entry.path]; !ok {
				continue
			}
			wanted[entry.path] = true
		}
		data, err := m.ws.Repo().Bytes(ctx, "cat-file", "blob", entry.blob)
		if err != nil {
			return Prepared{}, fmt.Errorf("read proposal %s: %w", entry.path, err)
		}
		proposals = append(proposals, ProposalInput{Path: entry.path, Blob: entry.blob})
		contents[entry.path] = data
	}
	for name, found := range wanted {
		if !found {
			return Prepared{}, fmt.Errorf("selected proposal not found: %s", name)
		}
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
		if canonical.IsMemoryPath(entry.path) && (entry.mode == "100644" || entry.mode == "100755") && entry.kind == "blob" {
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

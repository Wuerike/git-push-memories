package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/consolidation"
	"github.com/Wuerike/git-push-memories/internal/forge/github"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	inner         *mcp.Server
	retriever     *retrieval.Retriever
	cfg           config.Config
	inbox         *inbox.Inbox
	inboxMu       sync.Mutex
	consolidation *consolidation.Manager
}

func New(cfg config.Config, version string) (*Server, error) {
	if cfg.StateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	retriever, err := retrieval.New(cfg.MemoryRoot, cfg.MemoryCategories...)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, retriever: retriever, consolidation: consolidation.New(cfg), inner: mcp.NewServer(&mcp.Implementation{Name: "git-push-memories", Version: version}, nil)}
	mcp.AddTool(s.inner, &mcp.Tool{Name: "memory_propose", Description: "Commit a free Markdown proposal to the shared proposals branch through GitHub."}, s.addProposal)
	mcp.AddTool(s.inner, &mcp.Tool{Name: "memory_search", Description: "Search published memory text. Returns a Git revision and excerpts with paths and line intervals; use memory_read to investigate further."}, s.search)
	mcp.AddTool(s.inner, &mcp.Tool{Name: "memory_read", Description: "Read a published memory file or line window at a Git revision. Use the search revision for a consistent snapshot."}, s.read)
	mcp.AddTool(s.inner, &mcp.Tool{Name: "memory_proposal_list", Description: "List pending Markdown files with path, blob and content, plus the allowed categories from gpm.json. The agent interprets subjects and titles."}, s.listProposals)
	mcp.AddTool(s.inner, &mcp.Tool{Name: "memory_promote_prepare", Description: "Prepare a worktree from fetched main and fixed proposal inputs. Optionally select proposal paths. Returns categories, revisions and file inventories. Completes a previously merged publication first."}, s.preparePromotion)
	mcp.AddTool(s.inner, &mcp.Tool{Name: "memory_promote_check", Description: "Return changed memory paths and their Git diff without modifying files or the worktree index."}, s.checkPromotion)
	mcp.AddTool(s.inner, &mcp.Tool{Name: "memory_promote_submit", Description: "After human approval, commit selected changed memory paths, push the branch and open a PR with the approved summary."}, s.submitPromotion)
	mcp.AddTool(s.inner, &mcp.Tool{Name: "memory_promote_finish", Description: "Confirm merge, remove only selected proposals with matching path/blob, and refresh published memory."}, s.finishPromotion)
	return s, nil
}

func (s *Server) Run(ctx context.Context, input io.Reader, output io.Writer) error {
	defer s.retriever.Close()
	return s.inner.Run(ctx, &mcp.IOTransport{Reader: io.NopCloser(input), Writer: nopWriteCloser{Writer: output}})
}

type searchInput struct {
	Query      string `json:"query" jsonschema:"text terms to find; matches any term, case insensitive"`
	PathPrefix string `json:"path_prefix,omitempty" jsonschema:"optional relative path or directory"`
	Limit      int    `json:"limit,omitempty" jsonschema:"maximum matching files, default 20, up to 100"`
}

type readInput struct {
	Path      string `json:"path" jsonschema:"relative memory Markdown path"`
	Revision  string `json:"revision,omitempty" jsonschema:"Git revision, defaults to the published checkout HEAD; use the revision returned by search"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"first line, one based, default 1"`
	LineCount int    `json:"line_count,omitempty" jsonschema:"maximum lines, default 200, up to 2000"`
}

type addProposalInput struct {
	Title   string `json:"title" jsonschema:"proposal title used to name the file and commit"`
	Content string `json:"content" jsonschema:"free Markdown knowledge and relevant context for later consolidation"`
}

type emptyInput struct{}
type prepareInput struct {
	ProposalPaths []string `json:"proposal_paths,omitempty" jsonschema:"pending proposal paths to consolidate; omit to select all"`
}
type submitInput struct {
	Summary string   `json:"summary" jsonschema:"approved summary with destinations or exclusion justification for every selected proposal"`
	Paths   []string `json:"paths,omitempty" jsonschema:"reviewed changed memory paths to commit; omit to commit all changed memory paths"`
}

type toolError struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}
type response[T any] struct {
	SchemaVersion int        `json:"schema_version"`
	Data          *T         `json:"data,omitempty"`
	Error         *toolError `json:"error,omitempty"`
}

func respond[T any](data T, err error) (*mcp.CallToolResult, response[T], error) {
	if err == nil {
		return nil, response[T]{SchemaVersion: 1, Data: &data}, nil
	}
	return failure[T](errorCode(err), err.Error())
}
func failure[T any](code, message string) (*mcp.CallToolResult, response[T], error) {
	return &mcp.CallToolResult{IsError: true}, response[T]{SchemaVersion: 1, Error: &toolError{Code: code, Message: message}}, nil
}
func errorCode(err error) string {
	switch {
	case errors.Is(err, consolidation.ErrNoProposals):
		return "no_proposals"
	case errors.Is(err, consolidation.ErrNotPrepared):
		return "promotion_not_prepared"
	case errors.Is(err, consolidation.ErrNoChanges):
		return "no_changes"
	case errors.Is(err, consolidation.ErrSummary):
		return "invalid_summary"
	case errors.Is(err, memory.ErrInvalidPath):
		return "invalid_path"
	case errors.Is(err, inbox.ErrInvalidProposal):
		return "invalid_proposal"
	case errors.Is(err, workspace.ErrPermission), errors.Is(err, github.ErrPermission):
		return "push_denied"
	case errors.Is(err, workspace.ErrNetwork):
		return "network_unavailable"
	case errors.Is(err, workspace.ErrDirty):
		return "worktree_dirty"
	case errors.Is(err, github.ErrUnavailable), errors.Is(err, github.ErrNotGitHub):
		return "gh_unavailable"
	case errors.Is(err, retrieval.ErrInvalidQuery), errors.Is(err, retrieval.ErrInvalidScope), errors.Is(err, retrieval.ErrInvalidRange):
		return retrieval.ErrorCode(err)
	default:
		return "operation_failed"
	}
}

func (s *Server) search(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, response[retrieval.ContextBundle], error) {
	if _, err := s.cfg.Workspace().ResolveRead(); err != nil {
		return respond(retrieval.ContextBundle{}, err)
	}
	data, err := s.retriever.Retrieve(ctx, input.Query, retrieval.Scope{PathPrefix: input.PathPrefix}, input.Limit)
	return respond(data, err)
}
func (s *Server) read(ctx context.Context, _ *mcp.CallToolRequest, input readInput) (*mcp.CallToolResult, response[retrieval.FileContent], error) {
	if _, err := s.cfg.Workspace().ResolveRead(); err != nil {
		return respond(retrieval.FileContent{}, err)
	}
	data, err := s.retriever.Read(ctx, input.Path, input.Revision, input.StartLine, input.LineCount)
	return respond(data, err)
}

type proposalSummary struct {
	Path    string `json:"path"`
	Blob    string `json:"blob"`
	Content string `json:"content"`
}
type proposalListData struct {
	Categories []string          `json:"categories"`
	Proposals  []proposalSummary `json:"proposals"`
}

func (s *Server) listProposals(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, response[proposalListData], error) {
	box, err := s.proposalInbox()
	if err != nil {
		return respond(proposalListData{}, err)
	}
	_ = box.Refresh(ctx) // Offline reads use the last known committed inbox.
	snapshots, err := box.List(ctx)
	if err != nil {
		return respond(proposalListData{}, err)
	}
	data := proposalListData{Categories: append([]string(nil), s.cfg.MemoryCategories...), Proposals: []proposalSummary{}}
	for _, snapshot := range snapshots {
		p := snapshot.Proposal
		data.Proposals = append(data.Proposals, proposalSummary{Path: p.Path, Blob: p.Blob, Content: p.Content})
	}
	return respond(data, nil)
}
func (s *Server) addProposal(ctx context.Context, _ *mcp.CallToolRequest, input addProposalInput) (*mcp.CallToolResult, response[inbox.Proposal], error) {
	box, err := s.proposalInbox()
	if err != nil {
		return respond(inbox.Proposal{}, err)
	}
	proposal, err := box.Add(ctx, inbox.Request{Title: input.Title, Content: input.Content})
	return respond(proposal, err)
}
func (s *Server) preparePromotion(ctx context.Context, _ *mcp.CallToolRequest, input prepareInput) (*mcp.CallToolResult, response[consolidation.Prepared], error) {
	finished, err := s.consolidation.Finish(ctx)
	if err != nil {
		return respond(consolidation.Prepared{}, err)
	}
	if finished.Found && !finished.Merged {
		return failure[consolidation.Prepared]("merge_pending", fmt.Sprintf("PR %s is awaiting merge", finished.PRURL))
	}
	prepared, err := s.consolidation.Prepare(ctx, input.ProposalPaths...)
	return respond(prepared, err)
}
func (s *Server) checkPromotion(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, response[consolidation.Checked], error) {
	data, err := s.consolidation.Check(ctx)
	return respond(data, err)
}
func (s *Server) submitPromotion(ctx context.Context, _ *mcp.CallToolRequest, input submitInput) (*mcp.CallToolResult, response[consolidation.Submitted], error) {
	data, err := s.consolidation.Submit(ctx, input.Summary, input.Paths...)
	return respond(data, err)
}
func (s *Server) finishPromotion(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, response[consolidation.Finished], error) {
	data, err := s.consolidation.Finish(ctx)
	return respond(data, err)
}
func (s *Server) proposalInbox() (*inbox.Inbox, error) {
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	if s.inbox == nil {
		box, err := inbox.NewWriter(s.cfg.Workspace(), s.cfg.Paths.Proposals, workspace.ProposalsBranch)
		if err != nil {
			return nil, err
		}
		s.inbox = box
	}
	return s.inbox, nil
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

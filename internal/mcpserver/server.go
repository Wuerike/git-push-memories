package mcpserver

import (
	"context"
	"encoding/json"
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
	pages         *memory.Repository
	pagesMu       sync.Mutex
}

func New(cfg config.Config, version string) (*Server, error) {
	if cfg.StateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	retriever, err := retrieval.New(cfg.MemoryRoot, cfg.MemoryCategories...)
	if err != nil {
		return nil, err
	}
	server := &Server{cfg: cfg, retriever: retriever, consolidation: consolidation.New(cfg),
		inner: mcp.NewServer(&mcp.Implementation{Name: "git-push-memories", Version: version}, nil)}
	mcp.AddTool(server.inner, &mcp.Tool{Name: "memory_propose", Description: "Add one reviewable Markdown proposal to the proposals branch through GitHub, in a single call that either succeeds or changes nothing."}, server.addProposal)
	mcp.AddTool(server.inner, &mcp.Tool{Name: "memory_search", Description: "Search published memory in configured categories and return ranked section excerpts with expandable handles."}, server.search)
	mcp.AddTool(server.inner, &mcp.Tool{Name: "memory_expand", Description: "Expand one published memory section from a search handle within an exact byte budget."}, server.expand)
	mcp.AddTool(server.inner, &mcp.Tool{Name: "memory_page_read", Description: "Read a whole published canonical page with its content hash, or fail with budget_too_small; never truncates a page."}, server.readPage)
	mcp.AddTool(server.inner, &mcp.Tool{Name: "memory_proposal_list", Description: "List committed Markdown proposals and configured categories. This live inbox is separate from a promotion's fixed inputs."}, server.listProposals)
	mcp.AddTool(server.inner, &mcp.Tool{Name: "memory_promote_prepare", Description: "Prepare a new worktree from fetched main and fixed proposal inputs. Returns file paths and inventories for full reading and discussion before editing. Completes a previously merged publication first."}, server.preparePromotion)
	mcp.AddTool(server.inner, &mcp.Tool{Name: "memory_promote_check", Description: "Complete missing canonical IDs and return the diff of the current files for human review. No commit or publication occurs."}, server.checkPromotion)
	mcp.AddTool(server.inner, &mcp.Tool{Name: "memory_promote_submit", Description: "After explicit human approval of the current files and diff, commit only canonical changes, push the branch and open a PR into main with the approved consolidation summary."}, server.submitPromotion)
	mcp.AddTool(server.inner, &mcp.Tool{Name: "memory_promote_finish", Description: "Confirm the published PR was merged, remove only concluded proposals with matching content, and refresh published memory."}, server.finishPromotion)
	return server, nil
}

func (s *Server) Run(ctx context.Context, input io.Reader, output io.Writer) error {
	defer s.retriever.Close()
	defer s.closePages()
	transport := &mcp.IOTransport{Reader: io.NopCloser(input), Writer: nopWriteCloser{Writer: output}}
	return s.inner.Run(ctx, transport)
}

type searchInput struct {
	Query       string `json:"query" jsonschema:"retrieval query"`
	PathPrefix  string `json:"path_prefix,omitempty" jsonschema:"optional canonical relative path scope"`
	BudgetBytes int    `json:"budget_bytes" jsonschema:"maximum UTF-8 bytes for the ContextBundle JSON"`
	Source      string `json:"source,omitempty" jsonschema:"main (default, published memory)"`
}

type expandInput struct {
	Handle      retrieval.Handle `json:"handle" jsonschema:"handle returned by memory_search"`
	BudgetBytes int              `json:"budget_bytes" jsonschema:"maximum UTF-8 bytes for the ContextBundle JSON"`
	Source      string           `json:"source,omitempty" jsonschema:"main (default, published memory)"`
}

type pageReadInput struct {
	Path        string `json:"path" jsonschema:"canonical relative memory page path inside a configured category"`
	BudgetBytes int    `json:"budget_bytes" jsonschema:"maximum UTF-8 bytes for the page JSON data object"`
	Source      string `json:"source,omitempty" jsonschema:"main (default, published memory)"`
}

type pageReadData struct {
	Path        string `json:"path"`
	ContentHash string `json:"content_hash"`
	Markdown    string `json:"markdown"`
}

type pageReadResponse struct {
	SchemaVersion int           `json:"schema_version"`
	Data          *pageReadData `json:"data,omitempty"`
	Error         *toolError    `json:"error,omitempty"`
}

type addProposalInput struct {
	Title   string `json:"title" jsonschema:"concise human-readable proposal title; only names the file and the commit"`
	Content string `json:"content" jsonschema:"free Markdown for a future promoter to evaluate: the fact, its context, the suggested action (create, update or remove) and target page, and its origin if it matters"`
}

type listProposalsInput struct {
	Source string `json:"source,omitempty" jsonschema:"proposals (default, the shared inbox)"`
}

type inboxToolResponse struct {
	SchemaVersion int             `json:"schema_version"`
	Data          *inbox.Proposal `json:"data,omitempty"`
	Error         *toolError      `json:"error,omitempty"`
}

type toolError struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

type toolResponse struct {
	SchemaVersion int                      `json:"schema_version"`
	Data          *retrieval.ContextBundle `json:"data,omitempty"`
	Error         *toolError               `json:"error,omitempty"`
}

type proposalSummary struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Title   string `json:"title"`
	Content string `json:"content"`
}
type proposalListData struct {
	Categories []string          `json:"categories"`
	Proposals  []proposalSummary `json:"proposals"`
}
type proposalListResponse struct {
	SchemaVersion int               `json:"schema_version"`
	Data          *proposalListData `json:"data,omitempty"`
	Error         *toolError        `json:"error,omitempty"`
}

var errUnknownSource = errors.New("unknown source")

func (s *Server) sourceRoot(ctx context.Context, source string) (string, error) {
	if source != "" && source != "main" {
		return "", errUnknownSource
	}
	return s.cfg.Workspace().ResolveRead(ctx, false)
}
func sourceErrorCode(err error, fallback string) string {
	if errors.Is(err, errUnknownSource) {
		return "invalid_source"
	}
	return fallback
}
func (s *Server) retrieverFor(ctx context.Context, source string) (*retrieval.Retriever, func(), error) {
	if _, err := s.sourceRoot(ctx, source); err != nil {
		return nil, nil, err
	}
	return s.retriever, func() {}, nil
}
func (s *Server) pagesFor(ctx context.Context, source string) (*memory.Repository, func(), error) {
	if _, err := s.sourceRoot(ctx, source); err != nil {
		return nil, nil, err
	}
	repository, err := s.pageRepository()
	return repository, func() {}, err
}

func (s *Server) listProposals(ctx context.Context, _ *mcp.CallToolRequest, input listProposalsInput) (*mcp.CallToolResult, proposalListResponse, error) {
	if input.Source != "" && input.Source != "proposals" {
		return proposalListFailure("invalid_source")
	}
	box, err := s.proposalInbox()
	if err != nil {
		return proposalListFailure("promotion_unavailable")
	}
	_ = box.Refresh(ctx) // Existing live-inbox behavior: report last seen data if refresh fails.
	snapshots, err := box.List(ctx)
	if errors.Is(err, inbox.ErrWrongBranch) {
		return proposalListFailure("wrong_branch")
	}
	if err != nil {
		return proposalListFailure("promotion_failed")
	}
	listed := proposalListData{Categories: append([]string(nil), s.cfg.MemoryCategories...), Proposals: make([]proposalSummary, len(snapshots))}
	for i, snapshot := range snapshots {
		p := snapshot.Proposal
		listed.Proposals[i] = proposalSummary{ID: p.ID, Path: p.Path, Title: p.Title, Content: p.Content}
	}
	return nil, proposalListResponse{SchemaVersion: 1, Data: &listed}, nil
}

type emptyInput struct{}
type submitPromotionInput struct {
	Summary string `json:"summary" jsonschema:"approved Markdown summary accounting for every initial proposal with its destinations or free-form discard justification"`
}
type promotionToolResponse[T any] struct {
	SchemaVersion int        `json:"schema_version"`
	Data          *T         `json:"data,omitempty"`
	Error         *toolError `json:"error,omitempty"`
}

func promotionToolFailure[T any](code, message string) (*mcp.CallToolResult, promotionToolResponse[T], error) {
	return &mcp.CallToolResult{IsError: true}, promotionToolResponse[T]{SchemaVersion: 1, Error: &toolError{Code: code, Message: message}}, nil
}
func consolidationFailure[T any](err error) (*mcp.CallToolResult, promotionToolResponse[T], error) {
	code := "promotion_failed"
	switch {
	case errors.Is(err, consolidation.ErrNoProposals):
		code = "no_proposals"
	case errors.Is(err, consolidation.ErrNotPrepared):
		code = "promotion_not_prepared"
	case errors.Is(err, consolidation.ErrNoChanges):
		code = "no_changes"
	case errors.Is(err, consolidation.ErrSummary):
		code = "invalid_summary"
	case errors.Is(err, memory.ErrInvalidPath), errors.Is(err, memory.ErrInvalidSchema), errors.Is(err, memory.ErrInvalidID), errors.Is(err, memory.ErrSectionExists):
		code = "invalid_memory"
	case errors.Is(err, workspace.ErrPermission):
		code = "push_denied"
	case errors.Is(err, workspace.ErrNetwork):
		code = "network_unavailable"
	case errors.Is(err, workspace.ErrDirty):
		code = "worktree_dirty"
	case errors.Is(err, github.ErrUnavailable), errors.Is(err, github.ErrNotGitHub):
		code = "gh_unavailable"
	}
	return promotionToolFailure[T](code, err.Error())
}

func (s *Server) preparePromotion(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, promotionToolResponse[consolidation.Prepared], error) {
	finished, err := s.consolidation.Finish(ctx)
	if err != nil {
		return consolidationFailure[consolidation.Prepared](err)
	}
	if finished.Found && !finished.Merged {
		return promotionToolFailure[consolidation.Prepared]("merge_pending", fmt.Sprintf("PR %s is awaiting merge", finished.PRURL))
	}
	prepared, err := s.consolidation.Prepare(ctx)
	if err != nil {
		return consolidationFailure[consolidation.Prepared](err)
	}
	return nil, promotionToolResponse[consolidation.Prepared]{SchemaVersion: 1, Data: &prepared}, nil
}
func (s *Server) checkPromotion(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, promotionToolResponse[consolidation.Checked], error) {
	checked, err := s.consolidation.Check(ctx)
	if err != nil {
		return consolidationFailure[consolidation.Checked](err)
	}
	return nil, promotionToolResponse[consolidation.Checked]{SchemaVersion: 1, Data: &checked}, nil
}
func (s *Server) submitPromotion(ctx context.Context, _ *mcp.CallToolRequest, input submitPromotionInput) (*mcp.CallToolResult, promotionToolResponse[consolidation.Submitted], error) {
	submitted, err := s.consolidation.Submit(ctx, input.Summary)
	if err != nil {
		return consolidationFailure[consolidation.Submitted](err)
	}
	return nil, promotionToolResponse[consolidation.Submitted]{SchemaVersion: 1, Data: &submitted}, nil
}
func (s *Server) finishPromotion(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, promotionToolResponse[consolidation.Finished], error) {
	finished, err := s.consolidation.Finish(ctx)
	if err != nil {
		return consolidationFailure[consolidation.Finished](err)
	}
	return nil, promotionToolResponse[consolidation.Finished]{SchemaVersion: 1, Data: &finished}, nil
}

type nopWriteCloser struct{ io.Writer }

func (s *Server) search(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, toolResponse, error) {
	retriever, closeRetriever, err := s.retrieverFor(ctx, input.Source)
	if err != nil {
		return retrieverFailure(err)
	}
	defer closeRetriever()
	bundle, err := retriever.Retrieve(ctx, input.Query, retrieval.Scope{PathPrefix: input.PathPrefix}, input.BudgetBytes)
	return retrievalResponse(bundle, err)
}

func (s *Server) expand(ctx context.Context, _ *mcp.CallToolRequest, input expandInput) (*mcp.CallToolResult, toolResponse, error) {
	retriever, closeRetriever, err := s.retrieverFor(ctx, input.Source)
	if err != nil {
		return retrieverFailure(err)
	}
	defer closeRetriever()
	bundle, err := retriever.Expand(ctx, input.Handle, input.BudgetBytes)
	return retrievalResponse(bundle, err)
}

func retrieverFailure(err error) (*mcp.CallToolResult, toolResponse, error) {
	return &mcp.CallToolResult{IsError: true}, toolResponse{SchemaVersion: 1, Error: &toolError{Code: sourceErrorCode(err, retrieval.ErrorCode(err))}}, nil
}

// readPage returns the whole page or a typed error. Codes: invalid_budget, invalid_path
// (outside categories, missing, or not a canonical page path), invalid_page (schema), budget_too_small, page_read_failed.
func (s *Server) readPage(ctx context.Context, _ *mcp.CallToolRequest, input pageReadInput) (*mcp.CallToolResult, pageReadResponse, error) {
	if input.BudgetBytes <= 0 {
		return pageReadFailure("invalid_budget")
	}
	if !s.cfg.IsMemoryPath(input.Path) {
		return pageReadFailure("invalid_path")
	}
	repository, closeRepository, err := s.pagesFor(ctx, input.Source)
	if err != nil {
		return pageReadFailure(sourceErrorCode(err, "page_read_failed"))
	}
	defer closeRepository()
	page, err := repository.ReadPage(ctx, input.Path)
	if err != nil {
		switch {
		case errors.Is(err, memory.ErrInvalidPath):
			return pageReadFailure("invalid_path")
		case errors.Is(err, memory.ErrInvalidSchema):
			return pageReadFailure("invalid_page")
		}
		return pageReadFailure("page_read_failed")
	}
	data := pageReadData{Path: page.Path, ContentHash: page.ContentHash, Markdown: page.Markdown}
	encoded, err := json.Marshal(data)
	if err != nil {
		return pageReadFailure("page_read_failed")
	}
	if len(encoded) > input.BudgetBytes {
		return pageReadFailure("budget_too_small")
	}
	return nil, pageReadResponse{SchemaVersion: 1, Data: &data}, nil
}

func pageReadFailure(code string) (*mcp.CallToolResult, pageReadResponse, error) {
	return &mcp.CallToolResult{IsError: true}, pageReadResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func (s *Server) pageRepository() (*memory.Repository, error) {
	s.pagesMu.Lock()
	defer s.pagesMu.Unlock()
	if s.pages != nil {
		return s.pages, nil
	}
	repository, err := memory.Open(s.cfg.MemoryRoot)
	if err != nil {
		return nil, err
	}
	s.pages = repository
	return repository, nil
}

func (s *Server) closePages() {
	s.pagesMu.Lock()
	defer s.pagesMu.Unlock()
	if s.pages != nil {
		_ = s.pages.Close()
	}
}

func (s *Server) addProposal(ctx context.Context, _ *mcp.CallToolRequest, input addProposalInput) (*mcp.CallToolResult, inboxToolResponse, error) {
	proposalInbox, err := s.proposalInbox()
	if err != nil {
		return inboxFailure("proposal_unavailable")
	}
	stored, err := proposalInbox.Add(ctx, inbox.Request{Title: input.Title, Content: input.Content})
	switch {
	case errors.Is(err, inbox.ErrInvalidProposal):
		return inboxFailure("invalid_proposal")
	case errors.Is(err, github.ErrUnavailable), errors.Is(err, github.ErrNotGitHub):
		return inboxFailure("gh_unavailable")
	case errors.Is(err, workspace.ErrPermission):
		return inboxFailure("push_denied")
	case err != nil:
		return inboxFailure("proposal_failed")
	}
	return nil, inboxToolResponse{SchemaVersion: 1, Data: &stored}, nil
}

func (s *Server) proposalInbox() (*inbox.Inbox, error) {
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	if s.inbox != nil {
		return s.inbox, nil
	}
	proposalInbox, err := inbox.NewWriter(s.cfg.Workspace(), s.cfg.Paths.Proposals, workspace.ProposalsBranch)
	if err != nil {
		return nil, err
	}
	s.inbox = proposalInbox
	return proposalInbox, nil
}

func inboxFailure(code string) (*mcp.CallToolResult, inboxToolResponse, error) {
	return &mcp.CallToolResult{IsError: true}, inboxToolResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func proposalListFailure(code string) (*mcp.CallToolResult, proposalListResponse, error) {
	return &mcp.CallToolResult{IsError: true}, proposalListResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func retrievalResponse(bundle retrieval.ContextBundle, err error) (*mcp.CallToolResult, toolResponse, error) {
	if err == nil {
		return nil, toolResponse{SchemaVersion: 1, Data: &bundle}, nil
	}
	return &mcp.CallToolResult{IsError: true}, toolResponse{
		SchemaVersion: 1,
		Error:         &toolError{Code: retrieval.ErrorCode(err)},
	}, nil
}

func (nopWriteCloser) Close() error { return nil }

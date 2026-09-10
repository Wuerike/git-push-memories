package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/curation"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	inner     *mcp.Server
	retriever *retrieval.Retriever
	guard     *intake.Guard
	cfg       config.Config
	stateRoot string
	inbox     *inbox.Inbox
	inboxMu   sync.Mutex
	curator   *curation.Curator
	curatorMu sync.Mutex
	previews  map[string]curation.Preview
}

func New(cfg config.Config, version string) (*Server, error) {
	if cfg.StateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	retriever, err := retrieval.New(cfg.MemoryRoot, cfg.MemoryCategories...)
	if err != nil {
		return nil, err
	}
	guard, err := intake.New()
	if err != nil {
		_ = retriever.Close()
		return nil, err
	}
	server := &Server{
		retriever: retriever,
		guard:     guard,
		cfg:       cfg,
		stateRoot: cfg.StateRoot,
		inner: mcp.NewServer(&mcp.Implementation{
			Name:    "git-push-memories",
			Version: version,
		}, nil),
	}
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_propose",
		Description: "Add one reviewable Markdown proposal to the repository inbox and commit it on main.",
	}, server.addProposal)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_search",
		Description: "Search current memory pages recursively in configured categories and return ranked section excerpts with expandable handles.",
	}, server.search)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_expand",
		Description: "Expand one current memory section from a search handle within an exact byte budget.",
	}, server.expand)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_proposal_list",
		Description: "List committed Markdown proposals and configured memory categories for explicit human curation.",
	}, server.listProposals)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_curate_preview",
		Description: "Preview creating, replacing, or removing canonical memory, or discarding one proposal, without writing.",
	}, server.previewCuration)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_curate_apply",
		Description: "Apply one previously previewed curation after explicit human confirmation.",
	}, server.applyCuration)
	return server, nil
}

func (s *Server) Run(ctx context.Context, input io.Reader, output io.Writer) error {
	defer s.retriever.Close()
	defer s.closeInbox()
	defer s.closeCurator()
	transport := &mcp.IOTransport{
		Reader: io.NopCloser(input),
		Writer: nopWriteCloser{Writer: output},
	}
	return s.inner.Run(ctx, transport)
}

type searchInput struct {
	Query       string `json:"query" jsonschema:"retrieval query"`
	PathPrefix  string `json:"path_prefix,omitempty" jsonschema:"optional canonical relative path scope"`
	BudgetBytes int    `json:"budget_bytes" jsonschema:"maximum UTF-8 bytes for the ContextBundle JSON"`
}

type expandInput struct {
	Handle      retrieval.Handle `json:"handle" jsonschema:"handle returned by memory_search"`
	BudgetBytes int              `json:"budget_bytes" jsonschema:"maximum UTF-8 bytes for the ContextBundle JSON"`
}

type addProposalInput struct {
	Title           string   `json:"title" jsonschema:"concise human-readable proposal title"`
	SuggestedAction string   `json:"suggested_action" jsonschema:"suggested curation action: create, update, or remove"`
	Target          string   `json:"target,omitempty" jsonschema:"optional canonical relative memory target"`
	Knowledge       string   `json:"knowledge" jsonschema:"knowledge for a future curator to evaluate"`
	SourcePaths     []string `json:"source_paths" jsonschema:"unverified canonical relative source references supporting the proposal"`
}

type listProposalsInput struct{}

type previewCurationInput struct {
	Operation   string `json:"operation" jsonschema:"canonical operation: create_page, create_section, replace_section, remove_section, remove_page, or discard_proposal"`
	ProposalID  string `json:"proposal_id" jsonschema:"identifier of one committed proposal"`
	TargetPath  string `json:"target_path,omitempty" jsonschema:"canonical page path inside a configured memory category; omitted for discard_proposal"`
	SectionID   string `json:"section_id,omitempty" jsonschema:"leaf section identifier required by replace_section, remove_section, or remove_page"`
	ContentHash string `json:"content_hash,omitempty" jsonschema:"observed section content hash required by replace_section, remove_section, or remove_page"`
	Heading     string `json:"heading,omitempty" jsonschema:"new section heading required only by create_section"`
	Replacement string `json:"replacement,omitempty" jsonschema:"complete replacement body required only by replace_section"`
	Reason      string `json:"reason,omitempty" jsonschema:"discard reason required only by discard_proposal: not_relevant, incorrect, sensitive, wrong_target, or superseded"`
}

type applyCurationInput struct {
	PreviewToken string `json:"preview_token" jsonschema:"single-use token returned by memory_curate_preview"`
}

type curationPreview struct {
	PreviewToken  string                  `json:"preview_token"`
	RelatedMemory retrieval.ContextBundle `json:"related_memory"`
	curation.Preview
}

type proposalListResponse struct {
	SchemaVersion int            `json:"schema_version"`
	Data          *curation.List `json:"data,omitempty"`
	Error         *toolError     `json:"error,omitempty"`
}

type curationPreviewResponse struct {
	SchemaVersion int              `json:"schema_version"`
	Data          *curationPreview `json:"data,omitempty"`
	Error         *toolError       `json:"error,omitempty"`
}

type curationApplyResponse struct {
	SchemaVersion int              `json:"schema_version"`
	Data          *curation.Result `json:"data,omitempty"`
	Error         *toolError       `json:"error,omitempty"`
}

type inboxToolResponse struct {
	SchemaVersion int             `json:"schema_version"`
	Data          *inbox.Proposal `json:"data,omitempty"`
	Error         *toolError      `json:"error,omitempty"`
}

type toolError struct {
	Code string `json:"code"`
}

type toolResponse struct {
	SchemaVersion int                      `json:"schema_version"`
	Data          *retrieval.ContextBundle `json:"data,omitempty"`
	Error         *toolError               `json:"error,omitempty"`
}

func (s *Server) search(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, toolResponse, error) {
	bundle, err := s.retriever.Retrieve(ctx, input.Query, retrieval.Scope{PathPrefix: input.PathPrefix}, input.BudgetBytes)
	return retrievalResponse(bundle, err)
}

func (s *Server) addProposal(ctx context.Context, _ *mcp.CallToolRequest, input addProposalInput) (*mcp.CallToolResult, inboxToolResponse, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return inboxFailure("invalid_candidate")
	}
	allowed, err := s.guard.InspectProposal(intake.NewProposalCandidate(raw))
	if err != nil {
		var rejection intake.Rejection
		if errors.As(err, &rejection) {
			return inboxFailure(rejection.Class + "_" + rejection.Reason)
		}
		return inboxFailure("candidate_rejected")
	}
	if allowed.Target() != "" && !s.cfg.IsMemoryPath(allowed.Target()) {
		return inboxFailure("schema_invalid_target")
	}
	proposalInbox, err := s.proposalInbox()
	if err != nil {
		return inboxFailure("proposal_unavailable")
	}
	stored, err := proposalInbox.Add(ctx, inbox.Request{
		Title: allowed.Title(), SuggestedAction: inbox.Action(allowed.SuggestedAction()), Target: allowed.Target(),
		Knowledge: allowed.Knowledge(), SourcePaths: allowed.SourcePaths(),
	})
	if err != nil {
		if errors.Is(err, inbox.ErrNotMain) {
			return inboxFailure("not_on_main")
		}
		if errors.Is(err, inbox.ErrIndexLocked) {
			return inboxFailure("git_index_locked")
		}
		return inboxFailure("proposal_failed")
	}
	return nil, inboxToolResponse{SchemaVersion: 1, Data: &stored}, nil
}

func (s *Server) listProposals(ctx context.Context, _ *mcp.CallToolRequest, _ listProposalsInput) (*mcp.CallToolResult, proposalListResponse, error) {
	curator, err := s.proposalCurator()
	if err != nil {
		return proposalListFailure("curation_unavailable")
	}
	listed, err := curator.List(ctx)
	if err != nil {
		return proposalListFailure(curationErrorCode(err))
	}
	return nil, proposalListResponse{SchemaVersion: 1, Data: &listed}, nil
}

func (s *Server) previewCuration(ctx context.Context, _ *mcp.CallToolRequest, input previewCurationInput) (*mcp.CallToolResult, curationPreviewResponse, error) {
	curator, err := s.proposalCurator()
	if err != nil {
		return curationPreviewFailure("curation_unavailable")
	}
	preview, err := curator.Preview(ctx, curation.PreviewRequest{
		Operation: curation.Operation(input.Operation), ProposalID: input.ProposalID, TargetPath: input.TargetPath,
		SectionID: input.SectionID, ContentHash: input.ContentHash, Heading: input.Heading, Replacement: input.Replacement, Reason: curation.DiscardReason(input.Reason),
	})
	if err != nil {
		return curationPreviewFailure(curationErrorCode(err))
	}
	related, err := s.retriever.Retrieve(ctx, preview.Title+" "+preview.Knowledge, retrieval.Scope{}, 4096)
	if err != nil {
		return curationPreviewFailure(retrieval.ErrorCode(err))
	}
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return curationPreviewFailure("curation_unavailable")
	}
	token := hex.EncodeToString(tokenBytes)
	s.curatorMu.Lock()
	if s.previews == nil {
		s.previews = make(map[string]curation.Preview)
	}
	s.previews[token] = preview
	s.curatorMu.Unlock()
	response := curationPreview{PreviewToken: token, RelatedMemory: related, Preview: preview}
	return nil, curationPreviewResponse{SchemaVersion: 1, Data: &response}, nil
}

func (s *Server) applyCuration(ctx context.Context, _ *mcp.CallToolRequest, input applyCurationInput) (*mcp.CallToolResult, curationApplyResponse, error) {
	s.curatorMu.Lock()
	preview, found := s.previews[input.PreviewToken]
	delete(s.previews, input.PreviewToken)
	curator := s.curator
	s.curatorMu.Unlock()
	if !found || curator == nil {
		return curationApplyFailure("invalid_preview")
	}
	result, err := curator.Apply(ctx, preview)
	if err != nil {
		return curationApplyFailure(curationErrorCode(err))
	}
	return nil, curationApplyResponse{SchemaVersion: 1, Data: &result}, nil
}

func (s *Server) expand(ctx context.Context, _ *mcp.CallToolRequest, input expandInput) (*mcp.CallToolResult, toolResponse, error) {
	bundle, err := s.retriever.Expand(ctx, input.Handle, input.BudgetBytes)
	return retrievalResponse(bundle, err)
}

func (s *Server) closeInbox() {
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	if s.inbox != nil {
		_ = s.inbox.Close()
	}
}

func (s *Server) closeCurator() {
	s.curatorMu.Lock()
	defer s.curatorMu.Unlock()
	if s.curator != nil {
		_ = s.curator.Close()
	}
}

func (s *Server) proposalInbox() (*inbox.Inbox, error) {
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	if s.inbox != nil {
		return s.inbox, nil
	}
	proposalInbox, err := inbox.New(s.cfg.MemoryRoot, s.stateRoot)
	if err != nil {
		return nil, err
	}
	s.inbox = proposalInbox
	return proposalInbox, nil
}

func (s *Server) proposalCurator() (*curation.Curator, error) {
	s.curatorMu.Lock()
	defer s.curatorMu.Unlock()
	if s.curator != nil {
		return s.curator, nil
	}
	curator, err := curation.New(s.cfg)
	if err != nil {
		return nil, err
	}
	s.curator = curator
	return curator, nil
}

func inboxFailure(code string) (*mcp.CallToolResult, inboxToolResponse, error) {
	return &mcp.CallToolResult{IsError: true}, inboxToolResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func proposalListFailure(code string) (*mcp.CallToolResult, proposalListResponse, error) {
	return &mcp.CallToolResult{IsError: true}, proposalListResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func curationPreviewFailure(code string) (*mcp.CallToolResult, curationPreviewResponse, error) {
	return &mcp.CallToolResult{IsError: true}, curationPreviewResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func curationApplyFailure(code string) (*mcp.CallToolResult, curationApplyResponse, error) {
	return &mcp.CallToolResult{IsError: true}, curationApplyResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func curationErrorCode(err error) string {
	switch {
	case errors.Is(err, inbox.ErrNotMain):
		return "not_on_main"
	case errors.Is(err, inbox.ErrInvalidProposal):
		return "invalid_proposal"
	case errors.Is(err, curation.ErrUnsupported):
		return "unsupported_action"
	case errors.Is(err, curation.ErrLocked):
		return "git_index_locked"
	case errors.Is(err, curation.ErrInvalidReason):
		return "invalid_discard_reason"
	case errors.Is(err, inbox.ErrProposalDrift), errors.Is(err, curation.ErrDrift), errors.Is(err, memory.ErrDrift), errors.Is(err, memory.ErrTargetDirty), errors.Is(err, memory.ErrTargetExists):
		return "curation_drift"
	case errors.Is(err, memory.ErrInvalidPath), errors.Is(err, memory.ErrInvalidSchema), errors.Is(err, memory.ErrInvalidID), errors.Is(err, memory.ErrPageNotEligible),
		errors.Is(err, memory.ErrSectionExists), errors.Is(err, memory.ErrSectionNotFound), errors.Is(err, memory.ErrSectionNotLeaf):
		return "invalid_target"
	default:
		var rejection intake.Rejection
		if errors.As(err, &rejection) {
			return rejection.Class + "_" + rejection.Reason
		}
		return "curation_failed"
	}
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

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/inbox"
	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/memory"
	"github.com/Wuerike/git-push-memories/internal/promotion"
	"github.com/Wuerike/git-push-memories/internal/reference"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/Wuerike/git-push-memories/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	inner      *mcp.Server
	retriever  *retrieval.Retriever
	guard      *intake.Guard
	cfg        config.Config
	stateRoot  string
	inbox      *inbox.Inbox
	inboxMu    sync.Mutex
	promoter   *promotion.Promoter
	promoterMu sync.Mutex
	previews   map[string]pendingPromotion
	batches    map[string]pendingBatchPromotion
	references *reference.Verifier
	pages      *memory.Repository
	pagesMu    sync.Mutex
}

type pendingPromotion struct {
	Preview      promotion.Preview
	EvidenceHash [sha256.Size]byte
}

type pendingBatchPromotion struct {
	Preview        promotion.BatchPreview
	EvidenceHashes [][sha256.Size]byte
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
		retriever:  retriever,
		guard:      guard,
		references: reference.New(),
		cfg:        cfg,
		stateRoot:  cfg.StateRoot,
		inner: mcp.NewServer(&mcp.Implementation{
			Name:    "git-push-memories",
			Version: version,
		}, nil),
	}
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_propose",
		Description: "Add one reviewable Markdown proposal on the proposals branch and send it to the shared repository, or return an existing byte-identical proposal without adding a duplicate. pushed=false means it is committed locally and will be sent on the next call.",
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
		Name:        "memory_page_read",
		Description: "Read one whole canonical memory page with its content hash, or fail with budget_too_small; never returns a truncated page.",
	}, server.readPage)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_reference_snapshot",
		Description: "Snapshot immutable references to committed blobs in one explicit local Git repository without persisting its local root.",
	}, server.snapshotReferences)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_reference_expand",
		Description: "Read a bounded line range from the exact committed UTF-8 blob identified by a verified source reference without persisting content.",
	}, server.expandReference)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_proposal_list",
		Description: "List committed Markdown proposals and configured memory categories for explicit human promotion.",
	}, server.listProposals)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_promote_preview",
		Description: "Preview one promotion decision (create_page, create_section, replace_section, remove_section, remove_page, rewrite_page, discard_proposal, keep_pending) and its bounded canonical and inbox evidence without writing. fold_proposal is not accepted here. A new preview invalidates older pending tokens for the same proposals.",
	}, server.previewPromotion)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_promote_apply",
		Description: "Apply one previously previewed promotion after explicit human confirmation.",
	}, server.applyPromotion)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_promote_batch_preview",
		Description: "Preview a bounded batch of promotion decisions for one atomic commit, including fold_proposal items that fold other proposals into a rewrite_page or into a create_page that carries a replacement. A new preview invalidates older pending tokens for the same proposals.",
	}, server.previewPromotionBatch)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_promote_batch_apply",
		Description: "Apply one previously previewed promotion batch after explicit human confirmation.",
	}, server.applyPromotionBatch)
	return server, nil
}

func (s *Server) Run(ctx context.Context, input io.Reader, output io.Writer) error {
	defer s.retriever.Close()
	defer s.closeInbox()
	defer s.closePromoter()
	defer s.closePages()
	if _, err := promotion.Recover(ctx, s.cfg); err != nil {
		return err
	}
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

type pageReadInput struct {
	Path        string `json:"path" jsonschema:"canonical relative memory page path inside a configured category"`
	BudgetBytes int    `json:"budget_bytes" jsonschema:"maximum UTF-8 bytes for the page JSON data object"`
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
	Title            string                `json:"title" jsonschema:"concise human-readable proposal title"`
	SuggestedAction  string                `json:"suggested_action" jsonschema:"suggested promotion action: create, update, or remove"`
	Target           string                `json:"target,omitempty" jsonschema:"optional canonical relative memory target"`
	Knowledge        string                `json:"knowledge" jsonschema:"knowledge for a future promoter to evaluate"`
	SourcePaths      []string              `json:"source_paths" jsonschema:"unverified canonical relative source references supporting the proposal"`
	SourceReferences []reference.Reference `json:"source_references,omitempty" jsonschema:"immutable references returned by memory_reference_snapshot; source_paths must be empty"`
}

type snapshotReferencesInput struct {
	RepositoryRoot string   `json:"repository_root" jsonschema:"absolute local Git worktree root used transiently and never persisted"`
	Revision       string   `json:"revision,omitempty" jsonschema:"optional full commit object ID; defaults to HEAD"`
	SourcePaths    []string `json:"source_paths" jsonschema:"one to sixteen canonical repository-relative blob paths"`
}

type expandReferenceInput struct {
	Reference   reference.Reference `json:"reference" jsonschema:"source reference returned by memory_reference_snapshot"`
	StartLine   int                 `json:"start_line" jsonschema:"one-based first line to return"`
	BudgetBytes int                 `json:"budget_bytes" jsonschema:"maximum UTF-8 bytes for the encoded excerpt"`
}

type listProposalsInput struct {
	Source string `json:"source,omitempty" jsonschema:"where to list from: proposals (default, the shared inbox) or release (the open release, only for /promote)"`
}

type previewPromotionInput struct {
	Operation   string `json:"operation" jsonschema:"canonical operation: create_page, create_section, replace_section, remove_section, remove_page, rewrite_page, fold_proposal, discard_proposal, or keep_pending; fold_proposal is accepted only by memory_promote_batch_preview and must point to a rewrite_page item or to a create_page item that carries a replacement, in the same batch"`
	ProposalID  string `json:"proposal_id" jsonschema:"identifier of one committed proposal"`
	TargetPath  string `json:"target_path,omitempty" jsonschema:"canonical page path inside a configured memory category; omitted for fold_proposal and discard_proposal"`
	SectionID   string `json:"section_id,omitempty" jsonschema:"leaf section identifier required by replace_section, remove_section, or remove_page"`
	ContentHash string `json:"content_hash,omitempty" jsonschema:"observed content hash required by replace_section, remove_section, or remove_page; for rewrite_page, the content_hash returned by memory_page_read"`
	Heading     string `json:"heading,omitempty" jsonschema:"new section heading required only by create_section"`
	Replacement string `json:"replacement,omitempty" jsonschema:"complete replacement body required only by replace_section; for rewrite_page, the complete page Markdown; optional for create_page, the complete new page Markdown without frontmatter and without @id markers on H2 or deeper headings (the server generates them), which also lets the create_page receive fold_proposal items"`
	FoldInto    string `json:"fold_into,omitempty" jsonschema:"proposal_id of the rewrite_page item, or of the create_page item that carries a replacement, in the same batch that absorbs this proposal; required only by fold_proposal"`
	Reason      string `json:"reason,omitempty" jsonschema:"discard reason required only by discard_proposal: not_relevant, incorrect, sensitive, wrong_target, superseded, or duplicate"`
}

type applyPromotionInput struct {
	PreviewToken string `json:"preview_token" jsonschema:"single-use token returned by memory_promote_preview"`
}

type previewPromotionBatchInput struct {
	Items []previewPromotionInput `json:"items" jsonschema:"one to sixteen promotion decisions with distinct proposals; canonical items have distinct target paths, while fold_proposal items have no target path and depend on a rewrite_page or replacement-carrying create_page item in the same batch"`
}

type promotionPreview struct {
	PreviewToken string            `json:"preview_token"`
	Evidence     promotionEvidence `json:"evidence"`
	promotion.Preview
}

type promotionBatchItem struct {
	Evidence promotionEvidence `json:"evidence"`
	promotion.Preview
}

type promotionBatchPreview struct {
	PreviewToken string               `json:"preview_token"`
	Items        []promotionBatchItem `json:"items"`
	Diff         string               `json:"diff"`
}

type promotionEvidence struct {
	CanonicalCandidates retrieval.ContextBundle    `json:"canonical_candidates"`
	RelatedProposals    promotion.RelatedProposals `json:"related_proposals"`
	References          []reference.Evidence       `json:"references"`
}

type proposalListResponse struct {
	SchemaVersion int             `json:"schema_version"`
	Data          *promotion.List `json:"data,omitempty"`
	Error         *toolError      `json:"error,omitempty"`
}

type promotionPreviewResponse struct {
	SchemaVersion int               `json:"schema_version"`
	Data          *promotionPreview `json:"data,omitempty"`
	Error         *toolError        `json:"error,omitempty"`
}

type promotionApplyResponse struct {
	SchemaVersion int               `json:"schema_version"`
	Data          *promotion.Result `json:"data,omitempty"`
	Error         *toolError        `json:"error,omitempty"`
}

type promotionBatchPreviewResponse struct {
	SchemaVersion int                    `json:"schema_version"`
	Data          *promotionBatchPreview `json:"data,omitempty"`
	Error         *toolError             `json:"error,omitempty"`
}

type promotionBatchApplyResponse struct {
	SchemaVersion int                    `json:"schema_version"`
	Data          *promotion.BatchResult `json:"data,omitempty"`
	Error         *toolError             `json:"error,omitempty"`
}

type inboxToolResponse struct {
	SchemaVersion int             `json:"schema_version"`
	Data          *inbox.Proposal `json:"data,omitempty"`
	Pushed        bool            `json:"pushed"`
	Message       string          `json:"message,omitempty"`
	Error         *toolError      `json:"error,omitempty"`
}

type referenceSnapshotResponse struct {
	SchemaVersion int                 `json:"schema_version"`
	Data          *reference.Snapshot `json:"data,omitempty"`
	Error         *toolError          `json:"error,omitempty"`
}

type referenceExpandResponse struct {
	SchemaVersion int                `json:"schema_version"`
	Data          *reference.Excerpt `json:"data,omitempty"`
	Error         *toolError         `json:"error,omitempty"`
}

type toolError struct {
	Code string `json:"code"`
}

type toolResponse struct {
	SchemaVersion int                      `json:"schema_version"`
	Data          *retrieval.ContextBundle `json:"data,omitempty"`
	Error         *toolError               `json:"error,omitempty"`
}

// touchRead answers from the read worktree as it is and refreshes it in the background when the TTL expired.
func (s *Server) touchRead(ctx context.Context) {
	_, _ = s.cfg.Workspace().ResolveRead(ctx, false)
}

func (s *Server) search(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, toolResponse, error) {
	s.touchRead(ctx)
	bundle, err := s.retriever.Retrieve(ctx, input.Query, retrieval.Scope{PathPrefix: input.PathPrefix}, input.BudgetBytes)
	return retrievalResponse(bundle, err)
}

// readPage returns the whole page or a typed error. Codes: invalid_budget, invalid_path
// (outside categories, missing, or not a canonical page path), invalid_page (schema), budget_too_small, page_read_failed.
func (s *Server) readPage(ctx context.Context, _ *mcp.CallToolRequest, input pageReadInput) (*mcp.CallToolResult, pageReadResponse, error) {
	s.touchRead(ctx)
	if input.BudgetBytes <= 0 {
		return pageReadFailure("invalid_budget")
	}
	if !s.cfg.IsMemoryPath(input.Path) {
		return pageReadFailure("invalid_path")
	}
	repository, err := s.pageRepository()
	if err != nil {
		return pageReadFailure("page_read_failed")
	}
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
		Knowledge: allowed.Knowledge(), SourcePaths: allowed.SourcePaths(), SourceReferences: allowed.SourceReferences(),
	})
	if err != nil {
		if errors.Is(err, inbox.ErrWrongBranch) {
			return inboxFailure("wrong_branch")
		}
		if errors.Is(err, workspace.ErrDirty) {
			return inboxFailure("worktree_dirty")
		}
		return inboxFailure("proposal_failed")
	}
	pushed, err := proposalInbox.Publish(ctx)
	switch {
	case errors.Is(err, workspace.ErrPermission):
		return inboxFailure("push_denied")
	case err != nil:
		return inboxFailure("push_failed")
	}
	response := inboxToolResponse{SchemaVersion: 1, Data: &stored, Pushed: pushed}
	if !pushed {
		response.Message = "The proposal is committed locally and will be sent to the shared repository on the next /remember."
	}
	return nil, response, nil
}

func (s *Server) snapshotReferences(ctx context.Context, _ *mcp.CallToolRequest, input snapshotReferencesInput) (*mcp.CallToolResult, referenceSnapshotResponse, error) {
	snapshot, err := s.references.Snapshot(ctx, input.RepositoryRoot, input.Revision, input.SourcePaths)
	if err != nil {
		code := "invalid_reference"
		if errors.Is(err, reference.ErrInvalidRoot) {
			code = "invalid_repository_root"
		}
		return &mcp.CallToolResult{IsError: true}, referenceSnapshotResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
	}
	return nil, referenceSnapshotResponse{SchemaVersion: 1, Data: &snapshot}, nil
}

func (s *Server) expandReference(ctx context.Context, _ *mcp.CallToolRequest, input expandReferenceInput) (*mcp.CallToolResult, referenceExpandResponse, error) {
	excerpt, err := s.references.Expand(ctx, input.Reference, input.StartLine, input.BudgetBytes)
	if err != nil {
		code := "reference_expand_failed"
		switch {
		case errors.Is(err, reference.ErrIntegrityUnavailable):
			code = "reference_unavailable"
		case errors.Is(err, reference.ErrIntegrityMismatch):
			code = "reference_mismatch"
		case errors.Is(err, reference.ErrInvalidRange):
			code = "invalid_reference_range"
		case errors.Is(err, reference.ErrBudgetTooSmall):
			code = "budget_too_small"
		case errors.Is(err, reference.ErrNonTextBlob):
			code = "reference_not_text"
		}
		return &mcp.CallToolResult{IsError: true}, referenceExpandResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
	}
	return nil, referenceExpandResponse{SchemaVersion: 1, Data: &excerpt}, nil
}

func (s *Server) listProposals(ctx context.Context, _ *mcp.CallToolRequest, _ listProposalsInput) (*mcp.CallToolResult, proposalListResponse, error) {
	promoter, err := s.proposalPromoter()
	if err != nil {
		return proposalListFailure("promotion_unavailable")
	}
	listed, err := promoter.List(ctx)
	if err != nil {
		return proposalListFailure(promotionErrorCode(err))
	}
	return nil, proposalListResponse{SchemaVersion: 1, Data: &listed}, nil
}

func (s *Server) previewPromotion(ctx context.Context, _ *mcp.CallToolRequest, input previewPromotionInput) (*mcp.CallToolResult, promotionPreviewResponse, error) {
	promoter, err := s.proposalPromoter()
	if err != nil {
		return promotionPreviewFailure("promotion_unavailable")
	}
	preview, err := promoter.Preview(ctx, promotionRequest(input))
	if err != nil {
		return promotionPreviewFailure(promotionErrorCode(err))
	}
	evidence, err := s.buildPromotionEvidence(ctx, promoter, preview)
	if err != nil {
		return promotionPreviewFailure(promotionEvidenceErrorCode(err))
	}
	encodedEvidence, err := json.Marshal(evidence)
	if err != nil {
		return promotionPreviewFailure("promotion_unavailable")
	}
	evidenceHash := sha256.Sum256(encodedEvidence)
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return promotionPreviewFailure("promotion_unavailable")
	}
	token := hex.EncodeToString(tokenBytes)
	s.promoterMu.Lock()
	if s.previews == nil {
		s.previews = make(map[string]pendingPromotion)
	}
	s.invalidatePendingLocked(proposalIDs(preview))
	s.previews[token] = pendingPromotion{Preview: preview, EvidenceHash: evidenceHash}
	s.promoterMu.Unlock()
	response := promotionPreview{
		PreviewToken: token,
		Evidence:     evidence,
		Preview:      preview,
	}
	return nil, promotionPreviewResponse{SchemaVersion: 1, Data: &response}, nil
}

func (s *Server) applyPromotion(ctx context.Context, _ *mcp.CallToolRequest, input applyPromotionInput) (*mcp.CallToolResult, promotionApplyResponse, error) {
	s.promoterMu.Lock()
	pending, found := s.previews[input.PreviewToken]
	delete(s.previews, input.PreviewToken)
	promoter := s.promoter
	s.promoterMu.Unlock()
	if !found || promoter == nil {
		return promotionApplyFailure("invalid_preview")
	}
	result, err := promoter.Apply(ctx, pending.Preview, func() (string, error) {
		evidence, err := s.buildPromotionEvidence(ctx, promoter, pending.Preview)
		if err != nil {
			return "", promotion.ErrDrift
		}
		encodedEvidence, err := json.Marshal(evidence)
		if err != nil {
			return "", promotion.ErrDrift
		}
		hash := sha256.Sum256(encodedEvidence)
		if hash != pending.EvidenceHash {
			return "", promotion.ErrDrift
		}
		return "sha256:" + hex.EncodeToString(hash[:]), nil
	})
	if err != nil {
		return promotionApplyFailure(promotionErrorCode(err))
	}
	return nil, promotionApplyResponse{SchemaVersion: 1, Data: &result}, nil
}

func (s *Server) previewPromotionBatch(ctx context.Context, _ *mcp.CallToolRequest, input previewPromotionBatchInput) (*mcp.CallToolResult, promotionBatchPreviewResponse, error) {
	promoter, err := s.proposalPromoter()
	if err != nil {
		return promotionBatchPreviewFailure("promotion_unavailable")
	}
	requests := make([]promotion.PreviewRequest, len(input.Items))
	for index, item := range input.Items {
		requests[index] = promotionRequest(item)
	}
	preview, err := promoter.PreviewBatch(ctx, requests)
	if err != nil {
		return promotionBatchPreviewFailure(promotionErrorCode(err))
	}
	response := promotionBatchPreview{Items: make([]promotionBatchItem, len(preview.Items)), Diff: preview.Diff}
	hashes := make([][sha256.Size]byte, len(preview.Items))
	for index, item := range preview.Items {
		evidence, err := s.buildPromotionEvidence(ctx, promoter, item)
		if err != nil {
			return promotionBatchPreviewFailure(promotionEvidenceErrorCode(err))
		}
		encoded, err := json.Marshal(evidence)
		if err != nil {
			return promotionBatchPreviewFailure("promotion_unavailable")
		}
		hashes[index] = sha256.Sum256(encoded)
		response.Items[index] = promotionBatchItem{Preview: item, Evidence: evidence}
	}
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return promotionBatchPreviewFailure("promotion_unavailable")
	}
	response.PreviewToken = hex.EncodeToString(tokenBytes)
	s.promoterMu.Lock()
	if s.batches == nil {
		s.batches = make(map[string]pendingBatchPromotion)
	}
	ids := make([]string, len(preview.Items))
	for index, item := range preview.Items {
		ids[index] = item.ProposalID
	}
	s.invalidatePendingLocked(ids)
	s.batches[response.PreviewToken] = pendingBatchPromotion{Preview: preview, EvidenceHashes: hashes}
	s.promoterMu.Unlock()
	return nil, promotionBatchPreviewResponse{SchemaVersion: 1, Data: &response}, nil
}

func (s *Server) applyPromotionBatch(ctx context.Context, _ *mcp.CallToolRequest, input applyPromotionInput) (*mcp.CallToolResult, promotionBatchApplyResponse, error) {
	s.promoterMu.Lock()
	pending, found := s.batches[input.PreviewToken]
	delete(s.batches, input.PreviewToken)
	promoter := s.promoter
	s.promoterMu.Unlock()
	if !found || promoter == nil {
		return promotionBatchApplyFailure("invalid_preview")
	}
	result, err := promoter.ApplyBatch(ctx, pending.Preview, func() (string, error) {
		for index, preview := range pending.Preview.Items {
			evidence, err := s.buildPromotionEvidence(ctx, promoter, preview)
			if err != nil {
				return "", promotion.ErrDrift
			}
			encoded, err := json.Marshal(evidence)
			if err != nil || sha256.Sum256(encoded) != pending.EvidenceHashes[index] {
				return "", promotion.ErrDrift
			}
		}
		encodedHashes, err := json.Marshal(pending.EvidenceHashes)
		if err != nil {
			return "", promotion.ErrDrift
		}
		hash := sha256.Sum256(encodedHashes)
		return "sha256:" + hex.EncodeToString(hash[:]), nil
	})
	if err != nil {
		return promotionBatchApplyFailure(promotionErrorCode(err))
	}
	return nil, promotionBatchApplyResponse{SchemaVersion: 1, Data: &result}, nil
}

func proposalIDs(preview promotion.Preview) []string {
	return []string{preview.ProposalID}
}

// invalidatePendingLocked drops every pending single or batch token that
// contains any of the given proposals. A newer preview supersedes older ones
// for the same proposal. The caller must hold s.promoterMu.
func (s *Server) invalidatePendingLocked(ids []string) {
	shared := func(other ...string) bool {
		for _, id := range other {
			for _, candidate := range ids {
				if id == candidate {
					return true
				}
			}
		}
		return false
	}
	for token, pending := range s.previews {
		if shared(pending.Preview.ProposalID) {
			delete(s.previews, token)
		}
	}
	for token, pending := range s.batches {
		for _, item := range pending.Preview.Items {
			if shared(item.ProposalID) {
				delete(s.batches, token)
				break
			}
		}
	}
}

func promotionRequest(input previewPromotionInput) promotion.PreviewRequest {
	return promotion.PreviewRequest{
		Operation: promotion.Operation(input.Operation), ProposalID: input.ProposalID, TargetPath: input.TargetPath,
		SectionID: input.SectionID, ContentHash: input.ContentHash, Heading: input.Heading, Replacement: input.Replacement, FoldInto: input.FoldInto, Reason: promotion.DiscardReason(input.Reason),
	}
}

func (s *Server) buildPromotionEvidence(ctx context.Context, promoter *promotion.Promoter, preview promotion.Preview) (promotionEvidence, error) {
	query := preview.Title + " " + preview.Knowledge
	canonicalCandidates, err := s.retriever.Retrieve(ctx, query, retrieval.Scope{}, 4096)
	if err != nil {
		return promotionEvidence{}, err
	}
	relatedProposals, err := promoter.RelatedProposals(ctx, preview, query)
	if err != nil {
		return promotionEvidence{}, err
	}
	if err := promoter.RevalidatePreview(ctx, preview); err != nil {
		return promotionEvidence{}, err
	}
	references, err := s.references.Verify(ctx, preview.SourceReferences, preview.SourcePaths)
	if err != nil {
		return promotionEvidence{}, err
	}
	return promotionEvidence{
		CanonicalCandidates: canonicalCandidates,
		RelatedProposals:    relatedProposals,
		References:          references,
	}, nil
}

func (s *Server) expand(ctx context.Context, _ *mcp.CallToolRequest, input expandInput) (*mcp.CallToolResult, toolResponse, error) {
	s.touchRead(ctx)
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

func (s *Server) closePromoter() {
	s.promoterMu.Lock()
	defer s.promoterMu.Unlock()
	if s.promoter != nil {
		_ = s.promoter.Close()
	}
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

func (s *Server) proposalPromoter() (*promotion.Promoter, error) {
	s.promoterMu.Lock()
	defer s.promoterMu.Unlock()
	if s.promoter != nil {
		return s.promoter, nil
	}
	promoter, err := promotion.New(s.cfg, s.cfg.Paths.Proposals, workspace.ProposalsBranch)
	if err != nil {
		return nil, err
	}
	s.promoter = promoter
	return promoter, nil
}

func inboxFailure(code string) (*mcp.CallToolResult, inboxToolResponse, error) {
	return &mcp.CallToolResult{IsError: true}, inboxToolResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func proposalListFailure(code string) (*mcp.CallToolResult, proposalListResponse, error) {
	return &mcp.CallToolResult{IsError: true}, proposalListResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func promotionPreviewFailure(code string) (*mcp.CallToolResult, promotionPreviewResponse, error) {
	return &mcp.CallToolResult{IsError: true}, promotionPreviewResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func promotionApplyFailure(code string) (*mcp.CallToolResult, promotionApplyResponse, error) {
	return &mcp.CallToolResult{IsError: true}, promotionApplyResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func promotionBatchPreviewFailure(code string) (*mcp.CallToolResult, promotionBatchPreviewResponse, error) {
	return &mcp.CallToolResult{IsError: true}, promotionBatchPreviewResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func promotionBatchApplyFailure(code string) (*mcp.CallToolResult, promotionBatchApplyResponse, error) {
	return &mcp.CallToolResult{IsError: true}, promotionBatchApplyResponse{SchemaVersion: 1, Error: &toolError{Code: code}}, nil
}

func promotionErrorCode(err error) string {
	switch {
	case errors.Is(err, inbox.ErrWrongBranch):
		return "wrong_branch"
	case errors.Is(err, inbox.ErrInvalidProposal):
		return "invalid_proposal"
	case errors.Is(err, promotion.ErrUnsupported):
		return "unsupported_action"
	case errors.Is(err, promotion.ErrLocked):
		return "git_index_locked"
	case errors.Is(err, promotion.ErrInvalidReason):
		return "invalid_discard_reason"
	case errors.Is(err, promotion.ErrInvalidBatch):
		return "invalid_promotion_batch"
	case errors.Is(err, promotion.ErrRecovery):
		return "promotion_recovery_failed"
	case errors.Is(err, inbox.ErrProposalDrift), errors.Is(err, promotion.ErrDrift), errors.Is(err, memory.ErrDrift), errors.Is(err, memory.ErrTargetDirty), errors.Is(err, memory.ErrTargetExists):
		return "promotion_drift"
	case errors.Is(err, memory.ErrInvalidPath), errors.Is(err, memory.ErrInvalidSchema), errors.Is(err, memory.ErrInvalidID), errors.Is(err, memory.ErrPageNotEligible),
		errors.Is(err, memory.ErrSectionExists), errors.Is(err, memory.ErrSectionNotFound), errors.Is(err, memory.ErrSectionNotLeaf):
		return "invalid_target"
	default:
		var rejection intake.Rejection
		if errors.As(err, &rejection) {
			return rejection.Class + "_" + rejection.Reason
		}
		return "promotion_failed"
	}
}

func promotionEvidenceErrorCode(err error) string {
	switch {
	case errors.Is(err, retrieval.ErrInvalidQuery), errors.Is(err, retrieval.ErrInvalidScope), errors.Is(err, retrieval.ErrInvalidBudget),
		errors.Is(err, retrieval.ErrBudgetTooSmall), errors.Is(err, retrieval.ErrStaleHandle):
		return retrieval.ErrorCode(err)
	default:
		return promotionErrorCode(err)
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

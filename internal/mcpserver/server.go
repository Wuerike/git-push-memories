package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/intake"
	"github.com/Wuerike/git-push-memories/internal/proposal"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	inner      *mcp.Server
	retriever  *retrieval.Retriever
	guard      *intake.Guard
	cfg        config.Config
	stateRoot  string
	workflow   *proposal.Workflow
	workflowMu sync.Mutex
}

func New(cfg config.Config, stateRoot, version string) (*Server, error) {
	if stateRoot == "" {
		return nil, fmt.Errorf("state root is required")
	}
	retriever, err := retrieval.New(cfg.MemoryRoot)
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
		stateRoot: stateRoot,
		inner: mcp.NewServer(&mcp.Implementation{
			Name:    "git-push-memories",
			Version: version,
		}, nil),
	}
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_search",
		Description: "Search current Markdown memory and return ranked section excerpts with expandable handles.",
	}, server.search)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_expand",
		Description: "Expand one current memory section from a search handle within an exact byte budget.",
	}, server.expand)
	mcp.AddTool(server.inner, &mcp.Tool{
		Name:        "memory_propose_replace",
		Description: "Explicitly propose replacing one current memory section without applying or publishing it.",
	}, server.proposeReplace)
	return server, nil
}

func (s *Server) Run(ctx context.Context, input io.Reader, output io.Writer) error {
	defer s.retriever.Close()
	defer s.closeWorkflow()
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

type proposeInput struct {
	TargetPath  string   `json:"target_path" jsonschema:"canonical relative Markdown path in the memory repository"`
	SectionID   string   `json:"section_id" jsonschema:"stable section anchor ID"`
	Replacement string   `json:"replacement" jsonschema:"complete replacement body for the target section"`
	SourcePaths []string `json:"source_paths" jsonschema:"canonical relative code paths supporting the proposal"`
}

type toolError struct {
	Code string `json:"code"`
}

type toolResponse struct {
	SchemaVersion int                      `json:"schema_version"`
	Data          *retrieval.ContextBundle `json:"data,omitempty"`
	Error         *toolError               `json:"error,omitempty"`
}

type proposalToolResponse struct {
	SchemaVersion int               `json:"schema_version"`
	Data          *proposalToolData `json:"data,omitempty"`
	Error         *toolError        `json:"error,omitempty"`
}

type proposalToolData struct {
	ID          string          `json:"id"`
	Revision    int             `json:"revision"`
	Target      proposal.Target `json:"target"`
	DiffSummary diffSummary     `json:"diff_summary"`
}

type diffSummary struct {
	AddedLines   int `json:"added_lines"`
	RemovedLines int `json:"removed_lines"`
}

func (s *Server) search(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, toolResponse, error) {
	bundle, err := s.retriever.Retrieve(ctx, input.Query, retrieval.Scope{PathPrefix: input.PathPrefix}, input.BudgetBytes)
	return retrievalResponse(bundle, err)
}

func (s *Server) expand(ctx context.Context, _ *mcp.CallToolRequest, input expandInput) (*mcp.CallToolResult, toolResponse, error) {
	bundle, err := s.retriever.Expand(ctx, input.Handle, input.BudgetBytes)
	return retrievalResponse(bundle, err)
}

func (s *Server) proposeReplace(ctx context.Context, _ *mcp.CallToolRequest, input proposeInput) (*mcp.CallToolResult, proposalToolResponse, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return proposalFailure("invalid_candidate")
	}
	allowed, err := s.guard.Inspect(intake.NewCandidate(raw))
	if err != nil {
		var rejection intake.Rejection
		if errors.As(err, &rejection) {
			return proposalFailure(rejection.Class + "_" + rejection.Reason)
		}
		return proposalFailure("candidate_rejected")
	}
	workflow, err := s.proposalWorkflow()
	if err != nil {
		return proposalFailure("proposal_unavailable")
	}
	view, err := workflow.Prepare(ctx, allowed)
	if err != nil {
		return proposalFailure("proposal_failed")
	}
	return nil, proposalToolResponse{
		SchemaVersion: 1,
		Data: &proposalToolData{
			ID: view.ID, Revision: view.Revision, Target: view.Operation.Target,
			DiffSummary: summarizeDiff(view.Diff),
		},
	}, nil
}

func (s *Server) proposalWorkflow() (*proposal.Workflow, error) {
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	if s.workflow != nil {
		return s.workflow, nil
	}
	workflow, err := proposal.New(s.cfg.MemoryRoot, s.cfg.CodeRoot, s.stateRoot)
	if err != nil {
		return nil, err
	}
	s.workflow = workflow
	return workflow, nil
}

func (s *Server) closeWorkflow() {
	s.workflowMu.Lock()
	defer s.workflowMu.Unlock()
	if s.workflow != nil {
		_ = s.workflow.Close()
	}
}

func proposalFailure(code string) (*mcp.CallToolResult, proposalToolResponse, error) {
	return &mcp.CallToolResult{IsError: true}, proposalToolResponse{
		SchemaVersion: 1,
		Error:         &toolError{Code: code},
	}, nil
}

func summarizeDiff(diff string) diffSummary {
	var summary diffSummary
	for index, line := range strings.Split(diff, "\n") {
		if index < 3 || line == "" {
			continue
		}
		switch line[0] {
		case '+':
			summary.AddedLines++
		case '-':
			summary.RemovedLines++
		}
	}
	return summary
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

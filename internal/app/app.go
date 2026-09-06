package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/proposal"
	"github.com/Wuerike/git-push-memories/internal/protocol"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
)

const usage = `Usage:
  git-push-memories mcp --memory-root <path> --code-root <path>
  git-push-memories retrieve <query> --memory-root <path> --code-root <path> --budget-bytes <n>
  git-push-memories proposal show <proposal-id> --memory-root <path> --code-root <path> --state-root <path>
  git-push-memories proposal reject <proposal-id> --reason <reason> --memory-root <path> --code-root <path> --state-root <path>
  git-push-memories proposal apply <proposal-id> --revision <n> --memory-root <path> --code-root <path> --state-root <path>
  git-push-memories version
  git-push-memories help
`

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, version string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	case "version", "--version":
		fmt.Fprintf(stdout, "git-push-memories %s\n", version)
		return 0
	case "mcp":
		return runMCP(ctx, args[1:], stdin, stdout, stderr, version)
	case "retrieve":
		return runRetrieve(ctx, args[1:], stdout, stderr)
	case "proposal":
		return runProposal(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
}

func runProposal(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[0] != "show" && args[0] != "reject" && args[0] != "apply") {
		fmt.Fprintln(stderr, "proposal requires show, reject, or apply and a proposal ID")
		return 2
	}
	action, id := args[0], args[1]
	flags := flag.NewFlagSet("proposal "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	memoryRoot := flags.String("memory-root", "", "path to the memory Git worktree root")
	codeRoot := flags.String("code-root", "", "path to the code Git worktree root")
	stateRoot := flags.String("state-root", "", "path to private proposal state")
	reason := flags.String("reason", "", "closed rejection reason")
	revision := flags.Int("revision", 0, "exact proposal revision to apply")
	if err := flags.Parse(args[2:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 || (action == "show" && (*reason != "" || *revision != 0)) ||
		(action == "reject" && *revision != 0) || (action == "apply" && (*reason != "" || *revision < 1)) {
		fmt.Fprintln(stderr, "invalid proposal arguments")
		return 2
	}
	cfg, err := config.Load(*memoryRoot, *codeRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	var workflow *proposal.Workflow
	if action == "show" {
		workflow, err = proposal.OpenReadOnly(cfg.MemoryRoot, cfg.CodeRoot, *stateRoot)
	} else {
		workflow, err = proposal.New(cfg.MemoryRoot, cfg.CodeRoot, *stateRoot)
	}
	if err != nil {
		fmt.Fprintln(stderr, "proposal workflow could not initialize")
		return 1
	}
	defer workflow.Close()
	var response any
	if action == "show" {
		response, err = workflow.Review(id)
	} else if action == "reject" {
		response, err = workflow.Reject(id, *reason)
	} else {
		response, err = workflow.Apply(ctx, id, *revision)
	}
	if err != nil {
		switch {
		case errors.Is(err, proposal.ErrInvalidReason):
			fmt.Fprintln(stderr, "invalid_rejection_reason")
		case errors.Is(err, proposal.ErrNotFound):
			fmt.Fprintln(stderr, "proposal_not_found")
		case errors.Is(err, proposal.ErrWrongRevision):
			fmt.Fprintln(stderr, "proposal_revision_mismatch")
		case errors.Is(err, proposal.ErrTerminal):
			fmt.Fprintln(stderr, "proposal_already_terminal")
		case errors.Is(err, proposal.ErrIntegrity):
			fmt.Fprintln(stderr, "proposal_integrity_failed")
		default:
			fmt.Fprintln(stderr, "proposal_operation_failed")
		}
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(protocol.NewEnvelope(response)); err != nil {
		fmt.Fprintln(stderr, "write proposal response failed")
		return 1
	}
	return 0
}

func runRetrieve(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, "retrieve query is required")
		return 2
	}
	query := args[0]
	flags := flag.NewFlagSet("retrieve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	memoryRoot := flags.String("memory-root", "", "path to the memory Git worktree root")
	codeRoot := flags.String("code-root", "", "path to the code Git worktree root")
	budget := flags.Int("budget-bytes", 0, "maximum UTF-8 bytes for the ContextBundle JSON")
	pathPrefix := flags.String("path-prefix", "", "optional canonical relative path scope")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "retrieve accepts exactly one query")
		return 2
	}
	cfg, err := config.Load(*memoryRoot, *codeRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	retriever, err := retrieval.New(cfg.MemoryRoot)
	if err != nil {
		fmt.Fprintln(stderr, "retrieval could not initialize")
		return 1
	}
	defer retriever.Close()
	bundle, err := retriever.Retrieve(ctx, query, retrieval.Scope{PathPrefix: *pathPrefix}, *budget)
	if err != nil {
		fmt.Fprintln(stderr, retrieval.ErrorCode(err))
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(protocol.NewEnvelope(bundle)); err != nil {
		fmt.Fprintln(stderr, "write retrieval response failed")
		return 1
	}
	return 0
}

func runMCP(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, version string) int {
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	memoryRoot := flags.String("memory-root", "", "path to the memory Git worktree root")
	codeRoot := flags.String("code-root", "", "path to the code Git worktree root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "mcp does not accept positional arguments")
		return 2
	}

	cfg, err := config.Load(*memoryRoot, *codeRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	server, err := mcpserver.New(cfg, version)
	if err != nil {
		fmt.Fprintln(stderr, "MCP server could not initialize")
		return 1
	}
	if err := server.Run(ctx, stdin, stdout); err != nil {
		fmt.Fprintln(stderr, "MCP server stopped with a protocol or transport error")
		return 1
	}
	return 0
}

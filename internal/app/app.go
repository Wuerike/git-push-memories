package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/Wuerike/git-push-memories/internal/config"
	"github.com/Wuerike/git-push-memories/internal/mcpserver"
	"github.com/Wuerike/git-push-memories/internal/protocol"
	"github.com/Wuerike/git-push-memories/internal/retrieval"
)

const usage = `Usage:
  git-push-memories mcp --memory-root <path> --code-root <path>
  git-push-memories retrieve <query> --memory-root <path> --code-root <path> --budget-bytes <n>
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
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", args[0], usage)
		return 2
	}
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
